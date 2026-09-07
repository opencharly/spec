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
	"net"
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
	savedTimeout, savedRetries, savedBackoff, savedWaitDelay := gitOpMetadataTimeout, gitOpRetries, gitOpBackoff, gitOpWaitDelay
	gitOpMetadataTimeout = 300 * time.Millisecond
	gitOpRetries = 1
	gitOpBackoff = 30 * time.Millisecond
	gitOpWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() {
		gitOpMetadataTimeout, gitOpRetries, gitOpBackoff, gitOpWaitDelay = savedTimeout, savedRetries, savedBackoff, savedWaitDelay
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
	authOn   map[int]bool // respond 401 + WWW-Authenticate (the credential-challenge class)
	holdOn   map[int]bool // hijack the connection and hold it open FOREVER (the descendant-pipe class)
	holdAll  bool         // hold EVERY connection forever (the permanent-stall variant)
	conns    []net.Conn   // held connections, closed by releaseConns at test cleanup
	root     string       // GIT_PROJECT_ROOT
}

// releaseConns closes every connection the backend hijacked and held open
// forever, so a test server does not leak goroutines past the test.
func (g *gitBackend) releaseConns() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, c := range g.conns {
		_ = c.Close()
	}
	g.conns = nil
}

// record assigns the request ordinal and returns whether it must hang/drop/auth/hold.
func (g *gitBackend) recordAll() (n int, hang, drop, auth, hold bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests++
	n = g.requests
	return n, g.hangOn[n], g.dropOn[n], g.authOn[n], g.holdOn[n] || g.holdAll
}

// Count returns the number of served requests.
func (g *gitBackend) Count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.requests
}

func (g *gitBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, hang, drop, auth, hold := g.recordAll()
	if auth {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if hold {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		g.mu.Lock()
		g.conns = append(g.conns, conn)
		g.mu.Unlock()
		// Hold FOREVER: accept the request, never read to completion, never
		// respond, never close. Unlike hangOn (bounded server-side so the
		// deadline suffices), this is the production stall shape — libcurl has
		// no default response timeout, so ONLY the runner's deadline ends the
		// git process, and the surviving git-remote-http descendant keeps the
		// inherited pipes open. Bounded only if the runner is.
		return
	}
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

// TestGitOpBoundsSurvivingDescendantPipes is the 194-repo warmup hang: the
// server accepts the request and NEVER responds (libcurl has no default
// response timeout, so only the runner's deadline ends the git process).
// The deadline SIGKILLs git — but git's git-remote-http descendant survives
// (stuck in its own curl read), INHERITS the stdout/stderr pipe write-ends,
// and never closes them. exec.CommandContext kills only the DIRECT child;
// without WaitDelay, Output() waits for pipe EOF forever — the deadline is
// dead code and the WarmUp worker goroutine wedges until the process dies
// ("fetching git metadata for 194 repo(s)" followed by eternal silence; the
// r9 dead-socket freeze was this same shape). The runner must return in
// bounded time even when a descendant outlives the killed child.
func TestGitOpBoundsSurvivingDescendantPipes(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), holdOn: map[int]bool{1: true}}
	ts := httptest.NewServer(backend)
	defer ts.Close()
	t.Cleanup(backend.releaseConns)

	verdict := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := GitLatestTag(ts.URL + "/repo.git")
		verdict <- err
	}()
	select {
	case <-verdict:
		// Either the retry succeeded (the first held request consumed the
		// deadline; the second request was served) or it errored. BOTH are
		// correct — the regression is the PRE-fix behavior, where the op
		// NEVER returned at all because the surviving git-remote-http
		// descendant held the inherited pipes past the killed child.
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("GitLatestTag took %v against a never-responding server — not bounded", elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("GitLatestTag never returned: the deadline-killed git left git-remote-http holding the inherited pipes and Output() blocked forever — the 194-repo warmup hang")
	}
}

// TestGitOpBoundsPermanentDescendantHold is the sharpened form: EVERY
// connection is held open forever, so no retry can succeed — each attempt
// burns deadline + descendant-pipe grace and the op must still end with the
// deadline-reported error in bounded wall time ((retries+1) × (deadline +
// WaitDelay) + backoff), not hang forever.
func TestGitOpBoundsPermanentDescendantHold(t *testing.T) {
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), holdAll: true}
	ts := httptest.NewServer(backend)
	defer ts.Close()
	t.Cleanup(backend.releaseConns)

	start := time.Now()
	_, err := GitLatestTag(ts.URL + "/repo.git")
	if err == nil {
		t.Fatal("GitLatestTag against a permanently-held server must error")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %q, want it to report the deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("permanent descendant hold took %v — not bounded by deadline + WaitDelay", elapsed)
	}
}

// TestGitOpFailsFastOnCredentialChallenge: an auth-challenged repo (a private
// or renamed @github import) must fail FAST, not silently block on an
// interactive username/password prompt. The prompt is written to /dev/tty —
// invisible when runGitOp captures stderr — so a prompt would burn the full
// deadline ×(retries+1) per op per bad repo in a 194-repo warmup and read as
// a hang. The runner disables git's terminal prompts (GIT_TERMINAL_PROMPT=0,
// overriding any operator env), so the challenge is a permanent failure:
// exactly ONE request, no retry, bounded wall time — even when the parent
// env asks for prompts.
func TestGitOpFailsFastOnCredentialChallenge(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1") // simulate an operator env that wants prompts
	useFastGitOp(t)
	backend := &gitBackend{root: newBareGitRepo(t), authOn: map[int]bool{1: true}}
	ts := httptest.NewServer(backend)
	defer ts.Close()

	start := time.Now()
	_, err := GitLatestTag(ts.URL + "/repo.git")
	if err == nil {
		t.Fatal("GitLatestTag against a credential challenge must error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("credential challenge took %v — the runner blocked on interactive input", elapsed)
	}
	if n := backend.Count(); n != 1 {
		t.Fatalf("backend served %d requests, want exactly 1 (a credential challenge is permanent, not transient)", n)
	}
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
