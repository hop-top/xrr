package exec

import (
	"errors"
	osexec "os/exec"
)

// ExitCodeFromError extracts a process exit code from err.
//
// Returns:
//   - 0 if err is nil (success).
//   - The underlying process exit code if err is (or wraps) *os/exec.ExitError.
//   - -1 if err is non-nil but not a process exit error (e.g. start failure,
//     context cancellation, I/O error). Callers should treat -1 as
//     "unknown / not a clean process exit".
//
// Use this helper when wrapping an existing CommandRunner-style interface
// in an xrr session: the caller must populate Response.ExitCode but the
// upstream Run/RunInDir signatures only return (output, error).
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
