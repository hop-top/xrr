package xrr_test

import (
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
	require.NoError(t, c.Save("exec", fp, req, resp))
	var gotReq, gotResp map[string]any
	require.NoError(t, c.Load("exec", fp, &gotReq, &gotResp))
	assert.Equal(t, req, gotReq)
	assert.Equal(t, resp, gotResp)
}
