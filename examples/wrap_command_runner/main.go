// Package main demonstrates the canonical adoption pattern for xrr:
// wrapping an existing CommandRunner-style interface so it transparently
// records and replays through an xrr session, with zero call-site refactor
// in the consuming codebase.
//
// Pattern in three parts:
//
//  1. Real — the existing app interface (stable, in production).
//  2. Wrapper — satisfies Real but routes through an xrr session.
//  3. Caller — uses Real and never knows xrr exists.
//
// Used in the wild by hop.top/git's internal/xrrx and tlc's
// internal/flowtest. Adopt this shape any time you have a subprocess-
// running interface you want to make deterministic in tests.
package main

import (
	"context"
	"fmt"
	"log"
	osexec "os/exec"

	xrr "hop.top/xrr"
	xexec "hop.top/xrr/adapters/exec"
)

// Real is the existing app interface — stable, used everywhere in the
// consuming codebase. We do NOT change this. The whole point of the
// pattern is to leave call sites alone.
type Real interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	RunInDir(ctx context.Context, dir, name string, args ...string) (string, error)
}

// RealRunner is the production implementation of Real that shells out for
// real. In a real codebase this would already exist; reproduced here so
// the example is self-contained.
type RealRunner struct{}

func (RealRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := osexec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

func (RealRunner) RunInDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// Wrapper satisfies Real but routes every call through an xrr session.
// In ModeRecord it invokes the inner Real and persists the result.
// In ModeReplay it returns the cassette payload without invoking inner.
//
// Note: only the wrapper knows about xrr. Callers depend on Real.
type Wrapper struct {
	inner   Real
	sess    *xrr.FileSession
	adapter *xexec.Adapter
}

// NewWrapper wires inner + session + the exec adapter.
func NewWrapper(inner Real, sess *xrr.FileSession) *Wrapper {
	return &Wrapper{inner: inner, sess: sess, adapter: xexec.NewAdapter()}
}

// Run records or replays a `name args...` invocation.
//
// The natural shape works now that FileSession.record persists do()
// errors into the cassette envelope: just return whatever the inner
// runner returned. ExitCode is populated via ExitCodeFromError so the
// recorded payload still carries the real exit status; the error itself
// flows through xrr untouched and is re-emitted on replay.
func (w *Wrapper) Run(ctx context.Context, name string, args ...string) (string, error) {
	req := &xexec.Request{Argv: append([]string{name}, args...)}
	resp, err := w.sess.Record(ctx, w.adapter, req, func() (xrr.Response, error) {
		out, runErr := w.inner.Run(ctx, name, args...)
		return &xexec.Response{Stdout: out, ExitCode: xexec.ExitCodeFromError(runErr)}, runErr
	})
	return stdoutOf(resp), err
}

// RunInDir records or replays a `name args...` invocation in a working dir.
//
// Caveat: the current exec adapter fingerprint hashes only argv+stdin,
// so identical commands run in different dirs WILL collide on the same
// cassette. If your tests need per-directory isolation, either extend
// the adapter's fingerprinting inputs or namespace the cassette dir
// per test case. The dir is still honoured by inner.RunInDir during
// record mode — only the replay key is dir-agnostic.
func (w *Wrapper) RunInDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	req := &xexec.Request{Argv: append([]string{name}, args...)}
	resp, err := w.sess.Record(ctx, w.adapter, req, func() (xrr.Response, error) {
		out, runErr := w.inner.RunInDir(ctx, dir, name, args...)
		return &xexec.Response{Stdout: out, ExitCode: xexec.ExitCodeFromError(runErr)}, runErr
	})
	return stdoutOf(resp), err
}

// stdoutOf normalises the response: in record mode it's *xexec.Response;
// in replay mode it's *xrr.RawResponse with a YAML-decoded payload map.
// Both shapes need to look the same to callers of Real.
func stdoutOf(resp xrr.Response) string {
	if resp == nil {
		return ""
	}
	if r, ok := resp.(*xexec.Response); ok {
		return r.Stdout
	}
	if raw, ok := resp.(*xrr.RawResponse); ok {
		if s, ok := raw.Payload["stdout"].(string); ok {
			return s
		}
	}
	return ""
}

// Compile-time check: Wrapper satisfies Real.
var _ Real = (*Wrapper)(nil)

// ── Domain-specific runners ──────────────────────────────────────────────────
//
// In real codebases, the Real interface usually has a domain wrapper on top
// (GitRunner, DockerRunner, KubectlRunner, ...). The xrr Wrapper sits BELOW
// that wrapper — domain logic stays in production code, the xrr seam is
// pushed down to the lowest CommandRunner layer. This means:
//
//   - Domain wrappers stay testable via their own interface mocks.
//   - The xrr cassette captures the literal subprocess shape, not domain
//     concepts — replays survive refactors of GitRunner/DockerRunner.
//   - One Wrapper instance can back many domain wrappers in the same test.
//
// Below: GitRunner and DockerRunner, both built on Real.

// GitRunner exposes a tiny git surface backed by any Real implementation.
// In production it wraps RealRunner; in tests it wraps a record/replay
// Wrapper without changing GitRunner itself.
type GitRunner struct{ r Real }

// NewGitRunner constructs a GitRunner over r.
func NewGitRunner(r Real) *GitRunner { return &GitRunner{r: r} }

// CurrentBranch returns `git rev-parse --abbrev-ref HEAD` for repoDir.
func (g *GitRunner) CurrentBranch(ctx context.Context, repoDir string) (string, error) {
	out, err := g.r.RunInDir(ctx, repoDir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	return trimNewline(out), err
}

// Status returns short-format porcelain output for repoDir.
func (g *GitRunner) Status(ctx context.Context, repoDir string) (string, error) {
	return g.r.RunInDir(ctx, repoDir, "git", "status", "--porcelain")
}

// DockerRunner exposes a tiny docker surface backed by any Real.
type DockerRunner struct{ r Real }

// NewDockerRunner constructs a DockerRunner over r.
func NewDockerRunner(r Real) *DockerRunner { return &DockerRunner{r: r} }

// Version returns `docker version --format {{.Server.Version}}`.
func (d *DockerRunner) Version(ctx context.Context) (string, error) {
	out, err := d.r.Run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	return trimNewline(out), err
}

// Inspect returns the JSON blob from `docker inspect <ref>`.
func (d *DockerRunner) Inspect(ctx context.Context, ref string) (string, error) {
	return d.r.Run(ctx, "docker", "inspect", ref)
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func main() {
	ctx := context.Background()
	cassetteDir := "./_cassettes"

	// --- record pass: real subprocesses run, outputs captured
	rec := xrr.NewSession(xrr.ModeRecord, xrr.NewFileCassette(cassetteDir))
	w := NewWrapper(RealRunner{}, rec)

	// Plain echo through Wrapper.Run.
	out, err := w.Run(ctx, "echo", "hello from xrr")
	if err != nil {
		log.Fatalf("record echo: %v", err)
	}
	fmt.Printf("record echo: %s", out)

	// Git domain wrapper sitting on top of the same xrr seam.
	git := NewGitRunner(w)
	branch, err := git.CurrentBranch(ctx, ".")
	if err != nil {
		log.Printf("record git branch: %v (skipping if not in a git repo)", err)
	} else {
		fmt.Printf("record git branch: %s\n", branch)
	}

	// Docker domain wrapper. Both failure modes record cleanly now:
	// missing binary (start failure, ExitCodeFromError = -1) and
	// non-zero docker exit are persisted to the cassette and re-emitted
	// on replay with the same error string.
	docker := NewDockerRunner(w)
	ver, err := docker.Version(ctx)
	if err != nil {
		log.Printf("record docker version: %v (skipping if docker absent)", err)
	} else {
		fmt.Printf("record docker version: %s\n", ver)
	}

	// --- replay pass: same calls, but inner.Run is never invoked.
	// Domain wrappers are reconstructed over the replay-mode Wrapper —
	// no production code change needed.
	rep := xrr.NewSession(xrr.ModeReplay, xrr.NewFileCassette(cassetteDir))
	w2 := NewWrapper(RealRunner{}, rep)

	out, err = w2.Run(ctx, "echo", "hello from xrr")
	if err != nil {
		log.Fatalf("replay echo: %v", err)
	}
	fmt.Printf("replay echo: %s", out)

	git2 := NewGitRunner(w2)
	if branch, err := git2.CurrentBranch(ctx, "."); err == nil {
		fmt.Printf("replay git branch: %s\n", branch)
	}

	docker2 := NewDockerRunner(w2)
	if ver, err := docker2.Version(ctx); err == nil {
		fmt.Printf("replay docker version: %s\n", ver)
	}
}
