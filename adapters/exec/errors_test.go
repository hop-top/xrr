package exec_test

import (
	"context"
	"errors"
	"fmt"
	osexec "os/exec"
	"testing"
	"time"

	"hop.top/xrr/adapters/exec"

	"github.com/stretchr/testify/assert"
)

func TestExitCodeFromError_Nil(t *testing.T) {
	assert.Equal(t, 0, exec.ExitCodeFromError(nil))
}

func TestExitCodeFromError_ExitError(t *testing.T) {
	// `false` exits with 1 on every Unix.
	err := osexec.Command("false").Run()
	assert.Equal(t, 1, exec.ExitCodeFromError(err))
}

func TestExitCodeFromError_WrappedExitError(t *testing.T) {
	err := osexec.Command("false").Run()
	wrapped := fmt.Errorf("runner: %w", err)
	assert.Equal(t, 1, exec.ExitCodeFromError(wrapped))
}

func TestExitCodeFromError_NonExitError(t *testing.T) {
	// plain error is not a process exit
	assert.Equal(t, -1, exec.ExitCodeFromError(errors.New("boom")))
}

func TestExitCodeFromError_StartFailure(t *testing.T) {
	// command that does not exist → start failure, not ExitError
	err := osexec.Command("definitely-not-a-real-binary-xrr-test").Run()
	assert.Equal(t, -1, exec.ExitCodeFromError(err))
}

func TestExitCodeFromError_ContextCancel(t *testing.T) {
	// Generous timeout (200ms) to avoid flake on slow/loaded CI; the
	// command sleeps 30s so the timeout always wins regardless of jitter.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := osexec.CommandContext(ctx, "sleep", "30").Run()
	// killed by context → still surfaces as ExitError on most platforms;
	// either a real exit code or -1 is acceptable, but never 0.
	assert.NotEqual(t, 0, exec.ExitCodeFromError(err))
}
