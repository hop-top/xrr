package xrr_test

import (
	"errors"
	"testing"

	xrr "hop.top/xrr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCassetteSaveLoad(t *testing.T) {
	dir := t.TempDir()
	c := xrr.NewFileCassette(dir)
	// Use []any for argv — yaml round-trips []string as []any in map[string]any.
	req := map[string]any{"argv": []any{"gh", "pr", "view", "1"}}
	resp := map[string]any{"stdout": "title: foo", "exit_code": 0}
	fp := "a3f9c1b2"
	require.NoError(t, c.Save("exec", fp, req, resp, nil))
	var gotReq, gotResp map[string]any
	recordedErr, err := c.Load("exec", fp, &gotReq, &gotResp)
	require.NoError(t, err)
	assert.Empty(t, recordedErr)
	assert.Equal(t, req, gotReq)
	assert.Equal(t, resp, gotResp)
}

// TestFileCassetteSaveLoad_WithError — non-nil recordedErr round-trips
// through the envelope error field.
func TestFileCassetteSaveLoad_WithError(t *testing.T) {
	dir := t.TempDir()
	c := xrr.NewFileCassette(dir)
	req := map[string]any{"argv": []any{"false"}}
	resp := map[string]any{"stdout": "", "exit_code": 1}
	require.NoError(t, c.Save("exec", "deadbeef", req, resp, errors.New("exit status 1")))

	var gotReq, gotResp map[string]any
	recordedErr, err := c.Load("exec", "deadbeef", &gotReq, &gotResp)
	require.NoError(t, err)
	assert.Equal(t, "exit status 1", recordedErr)
	assert.Equal(t, req, gotReq)
	assert.Equal(t, resp, gotResp)
}
