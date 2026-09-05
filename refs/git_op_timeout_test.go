package refs

// git_op_timeout_test.go — the bounded git-op runner (refs/git.go runGitOp):
// every network git subprocess gets a context deadline plus a bounded retry,
// so a connection killed in a GitHub HTTP/2 reset window (curl error 92
// CANCEL / "RPC failed; HTTP/2 stream 5 reset") re-resolves instead of
// hanging forever (the r9 wave dead-socket freeze). These tests drive REAL
// git against a REAL smart-HTTP backend (git http-backend) whose Nth request
// hangs or drops the connection: the fetch must time out, retry, and succeed
// on the retry.

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// useFastGitOp shrinks the runner's deadline/backoff for the duration of a
// test so a hang is bounded in milliseconds, not seconds. The package's tests
// run sequentially (no t.Parallel anywhere in refs), so mutating the package
// vars is race-free.
func useFastGitOp(t *testing.T) {
	t.Helper()
	savedTimeout, savedRetries, savedBackoff := gitOpMetadataTimeout, gitOpRetries, gitOpBackoff
	gitOpMetadataTimeout = 300 * time.Millisecond
	gitOpRetries = 1
	gitOpBackoff = 30 * time.Millisecond
	t.Cleanup(func() {
		gitOpMetadataTimeout, gitOpRetries, gitOpBackoff = savedTimeout, savedRetries, savedBackoff
	})
}

// gitBackend serves a REAL git smart-HTTP backend (git http-backend — the same
// CGI the @github ref resolution speaks to) for bare repos under root, with
// per-request fault injection: hangOn connections are hijacked and left
// silent (git's libcurl blocks forever — only a deadline can end it), dropOn
// connections are killed mid-request (the curl-error-92 reset class).
type gitBackend struct {
	mu       sync.Mutex
	requests int
	hangOn   map[int]bool
	dropOn   map[int]bool
	root     string // GIT_PROJECT_ROOT
}

// record assigns the request ordinal and returns whether it must hang/drop.
func (g *gitBackend) record() (n int, hang, drop bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests++
	n = g.requests
	return n, g.hangOn[n], g.dropOn[n]
}

// Count returns the number of served requests.
func (g *gitBackend) Count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.requests
}

func (g *gitBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, hang, drop := g.record()
	if hang || drop {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if drop {
			return // kill the connection mid-request: the reset class
		}
		// Hang: hold the socket open without writing a single byte. git's
		// libcurl blocks on the response forever; only the runner's deadline
		// ends it. Bounded so an injected hang cannot outlive the test.
		time.Sleep(700 * time.Millisecond)
		return
	}
	runCGI(w, r, g.root)
}

// runCGI executes git http-backend as a CGI script, translating its stdout
// header block (CRLF-terminated, CGI style) into Go response headers.
func runCGI(w http.ResponseWriter, r *http.Request, projectRoot string) {
	cmd := exec.Command("git", "http-backend")
	cmd.Env = append(os.Environ(),
		"GIT_PROJECT_ROOT="+projectRoot,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO="+r.URL.Path,
		"QUERY_STRING="+r.URL.RawQuery,
		"REQUEST_METHOD="+r.Method,
		"CONTENT_TYPE="+r.Header.Get("Content-Type"),
	)
	cmd.Stdin = r.Body
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		http.Error(w, "git http-backend failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	raw := out.String()
	headers, body, _ := strings.Cut(raw, "\r\n\r\n")
	for _, line := range strings.Split(headers, "\r\n") {
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "Status: "); ok {
			if codeStr, _, _ := strings.Cut(rest, " "); codeStr != "" {
				if code, err := strconv.Atoi(strings.TrimSpace(codeStr)); err == nil {
					w.WriteHeader(code)
				}
			}
			continue
		}
		if k, v, ok := strings.Cut(line, ": "); ok {
			w.Header().Set(k, v)
		}
	}
	_, _ = w.Write([]byte(body))
}

// newBareGitRepo creates a bare repo (servable via git http-backend at
// <server>/repo.git) with two real empty commits tagged v1.0.0 and v2.0.0.
func newBareGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	mustGit(t, work, "init", "-q", "-b", "main")
	mustGit(t, work, "config", "user.name", "refs test")
	mustGit(t, work, "config", "user.email", "refs@test.local")
	mustGit(t, work, "commit", "--allow-empty", "-q", "-m", "init")
	mustGit(t, work, "tag", "v1.0.0")
	mustGit(t, work, "tag", "v2.0.0")
	bare := filepath.Join(dir, "repo.git")
	mustGit(t, "", "clone", "--bare", "-q", work, bare)
	// The CGI root is the PARENT — http-backend joins the URL path
	// ("/repo.git/info/refs") onto GIT_PROJECT_ROOT to find the bare repo.
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestGitOpRetriesAfterHang is the headline acceptance test: the FIRST
// connection to the ref server HANGS — the pre-fix behavior blocked forever
// (the r9 wave dead-socket freeze). The deadline kills git, the retry
// re-resolves against the live server, and GitLatestTag succeeds.
func TestGitOpRetriesAfterHang(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), hangOn: map[int]bool{1: true}}
	ts := httptest.NewServer(backend)
	defer ts.Close()

	tag, err := GitLatestTag(ts.URL + "/repo.git")
	if err != nil {
		t.Fatalf("GitLatestTag after hang-then-ok: %v", err)
	}
	if tag != "v2.0.0" {
		t.Fatalf("latest tag = %q, want v2.0.0", tag)
	}
	if n := backend.Count(); n != 2 {
		t.Fatalf("backend served %d requests, want 2 (one hang + one retry)", n)
	}
}

// TestGitOpRetriesAfterKilledConnection: the FIRST connection is killed
// mid-request (the curl error 92 CANCEL / HTTP/2-stream-reset class) — the
// retry re-resolves and succeeds.
func TestGitOpRetriesAfterKilledConnection(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), dropOn: map[int]bool{1: true}}
	ts := httptest.NewServer(backend)
	defer ts.Close()

	tag, err := GitLatestTag(ts.URL + "/repo.git")
	if err != nil {
		t.Fatalf("GitLatestTag after killed connection: %v", err)
	}
	if tag != "v2.0.0" {
		t.Fatalf("latest tag = %q, want v2.0.0", tag)
	}
	if n := backend.Count(); n != 2 {
		t.Fatalf("backend served %d requests, want 2 (one kill + one retry)", n)
	}
}

// TestGitOpBoundsPermanentHang: a server that hangs EVERY connection must not
// hang the caller — the deadline fires on each attempt, the error reports the
// timeout, and the wall time stays bounded by (retries+1) * deadline.
func TestGitOpBoundsPermanentHang(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), hangOn: map[int]bool{1: true, 2: true}}
	ts := httptest.NewServer(backend)
	defer ts.Close()

	start := time.Now()
	_, err := GitLatestTag(ts.URL + "/repo.git")
	if err == nil {
		t.Fatal("GitLatestTag against a permanently-hanging server must error")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %q, want it to report the deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("permanent hang took %v — not bounded by the deadline", elapsed)
	}
	if n := backend.Count(); n != 2 {
		t.Fatalf("backend served %d requests, want 2 (both attempts hung)", n)
	}
}

// TestGitOpDoesNotRetryPermanentFailure: a 404 (repo does not exist) is NOT a
// transient-network signature — it must fail fast with exactly ONE request.
func TestGitOpDoesNotRetryPermanentFailure(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t)}
	ts := httptest.NewServer(backend)
	defer ts.Close()

	_, err := GitLatestTag(ts.URL + "/missing.git")
	if err == nil {
		t.Fatal("GitLatestTag on a nonexistent repo must error")
	}
	if n := backend.Count(); n != 1 {
		t.Fatalf("backend served %d requests, want exactly 1 (404 is not retried)", n)
	}
}
