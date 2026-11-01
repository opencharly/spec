package refs

// git_transient_signatures_test.go — the RECORDED vocabulary of git's stderr,
// and the BEHAVIOUR the vocabulary must produce.
//
// The transient-fault vocabulary in refs/git.go decides whether a failed git
// subprocess is retried or given up on. It used to be one hand-written pattern,
// and a fault class nobody had written down — curl's "Error in the HTTP2
// framing layer", which git spells WITHOUT a slash — was classified
// PERMANENT: the bounded retry never fired, and one transient fault aborted a
// whole `charly marketplace generate` (opencharly/spec#284).
//
// This file is the RECORD: one observed stderr string per fault class, with
// where it was seen. The records are OFFLINE fixtures on purpose — the fault is
// transport-dependent and cannot be forced from inside the repo — so what is
// pinned is the DECISION the vocabulary makes about a string git has actually
// produced, not the network.
//
// It holds NO reference to the vocabulary's internals: the only production
// symbol it uses is runGitOp, which has existed unchanged since the runner was
// introduced. That is deliberate — it lets this file be run against ANY
// revision of refs/git.go, which is what makes it the RED proof: against the
// revision before the vocabulary was fixed, the HTTP/2 framing fault is the ONE
// recorded fault that is not retried.
//
// The vocabulary's own internals — the class names, and the rule that every
// class must be backed by a record here — are asserted in
// git_transient_classes_test.go.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// recordedTransientFault is one OBSERVED stderr string of a transient network
// fault: the fault class it belongs to, the string itself, and where it was
// seen.
type recordedTransientFault struct {
	class  string
	stderr string
	seen   string
}

// recordedPermanentFailure is one OBSERVED stderr string of a PERMANENT
// failure: retrying it cannot help, so it must fail fast with ONE attempt and
// must never be reported as a transient network fault.
type recordedPermanentFailure struct {
	stderr string
	seen   string
}

// recordedTransientFaults — the observed transient fault classes, one record
// each. Every class name here must exist in the vocabulary, and every class in
// the vocabulary must have a record here.
var recordedTransientFaults = []recordedTransientFault{
	{
		class:  "rpc failed",
		stderr: "error: RPC failed; HTTP 502 curl 22 The requested URL returned error: 502",
		seen:   "git's git-remote-http when the RPC stream is reset mid-transfer",
	},
	{
		class:  "http2 protocol fault",
		stderr: "fatal: unable to access 'https://github.com/opencharly/layer-golang.git/': Error in the HTTP2 framing layer",
		seen:   "opencharly/marketplace run 40922093306, 2026-10-29 — curl's framing fault, spelled WITHOUT a slash",
	},
	{
		class:  "connect failed",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Failed to connect to github.com port 443 after 21 ms: Connection refused",
		seen:   "curl's connect error against a refusing endpoint",
	},
	{
		class:  "connection reset",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Recv failure: Connection reset by peer",
		seen:   "curl's receive error when the peer resets the connection",
	},
	{
		class:  "host unresolvable",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Could not resolve host: github.com",
		seen:   "the resolver's DNS failure",
	},
	{
		class:  "deadline exceeded",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Operation timed out after 30001 milliseconds with 0 bytes received",
		seen:   "curl's own transfer timeout, distinct from the runner's deadline",
	},
	{
		class:  "early eof",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Early EOF",
		seen:   "curl's early end of stream",
	},
	{
		class:  "network unreachable",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Network is unreachable",
		seen:   "the kernel's no-route answer",
	},
	{
		class:  "transfer closed",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': transfer closed with outstanding read data remaining",
		seen:   "curl's half-finished transfer",
	},
	{
		class:  "hung up",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': The remote end hung up unexpectedly",
		seen:   "git's sideband end, on a connection that died",
	},
	{
		class:  "unexpected disconnect",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Unexpected disconnect while reading sideband packet",
		seen:   "git's sideband reader losing the stream",
	},
	{
		class:  "empty reply",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': Empty reply from server",
		seen:   "curl's empty response",
	},
	{
		class:  "server error",
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': The requested URL returned error: 502",
		seen:   "the server answering 5xx — the class that must stay retryable",
	},
}

// recordedPermanentFailures — the observed PERMANENT failures. None of them
// may be retried: a permanent failure must fail fast, with ONE attempt, and
// its real error must not be masked by a retry.
var recordedPermanentFailures = []recordedPermanentFailure{
	{
		stderr: "fatal: repository 'https://github.com/opencharly/does-not-exist.git/' not found",
		seen:   "a repository that does not exist",
	},
	{
		stderr: "fatal: unable to access 'https://github.com/opencharly/private.git/': The requested URL returned error: 403",
		seen:   "a repository the caller may not read",
	},
	{
		stderr: "fatal: unable to access 'https://github.com/opencharly/private.git/': The requested URL returned error: 404",
		seen:   "a repository that is gone, or was never there",
	},
	{
		stderr: "fatal: unable to access 'https://github.com/opencharly/x.git/': could not read Username for 'https://github.com': terminal prompts disabled",
		seen:   "the credential challenge — a permanent failure, never an interactive block",
	},
}

// installFakeGitAt puts a `git` on PATH that writes stderr verbatim and exits
// 128, and returns the path of the file it counts its invocations in. It
// stands in for the real git so the RETRY decision can be observed without a
// network: the runner's whole contract is "retry a transient fault, give up on
// a permanent one", and the invocation count is that contract, measured.
//
// The stderr is delivered through a FILE, not through the script's text, so a
// recorded string containing quotes, backslashes or a dollar sign is delivered
// byte for byte instead of being re-read by a shell.
func installFakeGitAt(t *testing.T, dir string, stderr string) string {
	t.Helper()
	msgPath := filepath.Join(dir, "stderr")
	countPath := filepath.Join(dir, "count")
	if err := os.WriteFile(msgPath, []byte(stderr+"\n"), 0o600); err != nil {
		t.Fatalf("write the recorded stderr: %v", err)
	}
	script := "#!/bin/sh\ncat " + strconv.Quote(msgPath) + " >&2\necho 1 >> " + strconv.Quote(countPath) + "\nexit 128\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write the fake git: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return countPath
}

// countFakeGitCalls reports how many times the fake git was invoked. The count
// is read from the counter file, because the fake git runs as a SEPARATE
// process and the counter file is the only thing that crosses that boundary.
func countFakeGitCalls(t *testing.T, countPath string) int {
	t.Helper()
	raw, err := os.ReadFile(countPath)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(raw)))
}

// TestRecordedTransientFaultsAreRetried: every OBSERVED transient fault is
// RETRIED. This is the coverage the vocabulary never had — it was written by
// hand and nothing checked it against a string git had actually produced, so a
// fault class nobody wrote down was given up on instead of retried.
func TestRecordedTransientFaultsAreRetried(t *testing.T) {
	useFastGitOp(t)
	for _, f := range recordedTransientFaults {
		t.Run(f.class, func(t *testing.T) {
			dir := t.TempDir()
			countPath := installFakeGitAt(t, dir, f.stderr)
			_, err := runGitOp(false, "", nil, "ls-remote", "https://github.com/opencharly/x.git", "v1")
			if err == nil {
				t.Fatal("the fake git failed, so runGitOp must report an error")
			}
			if calls := countFakeGitCalls(t, countPath); calls < 2 {
				t.Errorf("the recorded transient fault %q was attempted %d time(s) — a transient fault must be retried (seen in: %s)",
					f.stderr, calls, f.seen)
			}
		})
	}
}

// TestRecordedPermanentFailuresAreNotRetried: every OBSERVED permanent failure
// fails fast with exactly ONE attempt. Widening the vocabulary must not turn a
// permanent failure into a retried one — that would mask the real error and
// triple its latency.
func TestRecordedPermanentFailuresAreNotRetried(t *testing.T) {
	useFastGitOp(t)
	for _, f := range recordedPermanentFailures {
		t.Run(f.stderr, func(t *testing.T) {
			dir := t.TempDir()
			countPath := installFakeGitAt(t, dir, f.stderr)
			_, err := runGitOp(false, "", nil, "ls-remote", "https://github.com/opencharly/x.git", "v1")
			if err == nil {
				t.Fatal("the fake git failed, so runGitOp must report an error")
			}
			if calls := countFakeGitCalls(t, countPath); calls != 1 {
				t.Errorf("the recorded permanent failure %q was attempted %d time(s) — a permanent failure must fail fast with exactly one attempt (seen in: %s)",
					f.stderr, calls, f.seen)
			}
		})
	}
}
