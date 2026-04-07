package xrr

import (
	"fmt"
	"os"
)

// Env var names read by SessionFromEnv.
//
// These are the canonical names for cross-process e2e adoption: a test
// harness sets them before exec'ing a child binary, the child calls
// SessionFromEnv() at startup, and the child writes/reads cassettes
// from a directory the parent controls. See README "Cross-process
// e2e" section and examples/wrap_command_runner.
const (
	// EnvMode selects the session mode. Accepted values:
	// "record", "replay", "passthrough". Unset or empty ⇒ no session
	// (SessionFromEnv returns nil, nil).
	EnvMode = "XRR_MODE"

	// EnvCassetteDir is the file cassette directory. Required when
	// EnvMode is set to "record" or "replay". Unused for "passthrough".
	EnvCassetteDir = "XRR_CASSETTE_DIR"
)

// SessionFromEnv constructs a *FileSession from the XRR_MODE and
// XRR_CASSETTE_DIR environment variables, or returns (nil, nil) when
// XRR_MODE is unset.
//
// This is the canonical entry point for cross-process e2e adoption:
// a test harness sets the env vars before exec'ing a child binary, the
// child calls SessionFromEnv() at startup, and the child's internal
// xrr-wrapped runners all share one parent-controlled cassette
// directory. Callers that get (nil, nil) should fall back to their
// normal, non-recorded execution path.
//
// Returned errors:
//   - XRR_MODE set to an unrecognized value
//   - XRR_MODE set to "record" or "replay" but XRR_CASSETTE_DIR empty
//   - XRR_MODE set to "record" and XRR_CASSETTE_DIR refers to an
//     existing path that is not a directory
func SessionFromEnv() (*FileSession, error) {
	modeStr := os.Getenv(EnvMode)
	if modeStr == "" {
		return nil, nil
	}

	mode := Mode(modeStr)
	switch mode {
	case ModeRecord, ModeReplay, ModePassthrough:
		// ok
	default:
		return nil, fmt.Errorf("xrr: %s=%q is not a valid mode (want record|replay|passthrough)", EnvMode, modeStr)
	}

	if mode == ModePassthrough {
		// Passthrough bypasses the cassette entirely, so XRR_CASSETTE_DIR
		// is irrelevant. Still construct a session so callers see a
		// uniform API.
		return NewSession(mode, nil), nil
	}

	dir := os.Getenv(EnvCassetteDir)
	if dir == "" {
		return nil, fmt.Errorf("xrr: %s=%q requires %s to be set", EnvMode, modeStr, EnvCassetteDir)
	}

	switch mode {
	case ModeRecord:
		// Create the dir up front. FileCassette.Save writes files with
		// os.WriteFile, which does NOT create parents, so a missing
		// XRR_CASSETTE_DIR would otherwise fail at first save with a
		// less actionable OS error. If the path exists already, it
		// MUST be a directory.
		info, err := os.Stat(dir)
		switch {
		case err == nil:
			if !info.IsDir() {
				return nil, fmt.Errorf("xrr: %s=%q is not a directory", EnvCassetteDir, dir)
			}
		case os.IsNotExist(err):
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("xrr: failed to create %s=%q: %w", EnvCassetteDir, dir, err)
			}
		default:
			return nil, fmt.Errorf("xrr: failed to stat %s=%q: %w", EnvCassetteDir, dir, err)
		}
	case ModeReplay:
		// Replay reads cassettes, so the dir MUST exist and MUST be a
		// directory. Failing fast here gives a config error at startup
		// instead of a downstream cassette-miss that's harder to
		// attribute to a misconfigured env var.
		info, err := os.Stat(dir)
		switch {
		case err == nil:
			if !info.IsDir() {
				return nil, fmt.Errorf("xrr: %s=%q is not a directory", EnvCassetteDir, dir)
			}
		case os.IsNotExist(err):
			return nil, fmt.Errorf("xrr: %s=%q does not exist (replay mode requires an existing cassette directory)", EnvCassetteDir, dir)
		default:
			return nil, fmt.Errorf("xrr: failed to stat %s=%q: %w", EnvCassetteDir, dir, err)
		}
	}

	return NewSession(mode, NewFileCassette(dir)), nil
}
