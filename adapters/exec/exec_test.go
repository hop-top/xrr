package exec_test

import (
	"testing"

	"hop.top/xrr/adapters/exec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecAdapterFingerprint(t *testing.T) {
	a := exec.NewAdapter()
	req := &exec.Request{Argv: []string{"gh", "pr", "view", "1"}, Stdin: ""}
	fp, err := a.Fingerprint(req)
	require.NoError(t, err)
	assert.Len(t, fp, 8)
	// deterministic
	fp2, _ := a.Fingerprint(req)
	assert.Equal(t, fp, fp2)
	// different argv → different fp
	req2 := &exec.Request{Argv: []string{"gh", "pr", "view", "2"}}
	fp3, _ := a.Fingerprint(req2)
	assert.NotEqual(t, fp, fp3)
}

func TestExecAdapterRoundtrip(t *testing.T) {
	a := exec.NewAdapter()
	req := &exec.Request{Argv: []string{"echo", "hello"}, Stdin: ""}
	data, err := a.Serialize(req)
	require.NoError(t, err)
	var got exec.Request
	require.NoError(t, a.Deserialize(data, &got))
	assert.Equal(t, req.Argv, got.Argv)
}
