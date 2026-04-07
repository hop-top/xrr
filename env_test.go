package xrr_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	xrr "hop.top/xrr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionFromEnv_Unset(t *testing.T) {
	t.Setenv(xrr.EnvMode, "")
	t.Setenv(xrr.EnvCassetteDir, "")

	sess, err := xrr.SessionFromEnv()
	require.NoError(t, err)
	assert.Nil(t, sess, "unset XRR_MODE must return (nil, nil) so callers can fall back")
}

func TestSessionFromEnv_Record(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(xrr.EnvMode, string(xrr.ModeRecord))
	t.Setenv(xrr.EnvCassetteDir, dir)

	sess, err := xrr.SessionFromEnv()
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Exercise it end-to-end to prove it's wired correctly.
	adapter := &fakeAdapter{id: "exec", fp: "envrec01"}
	req := &fakeReq{}
	_, err = sess.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return &fakeResp{out: "hi"}, nil
	})
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "record mode must persist req + resp files into XRR_CASSETTE_DIR")
}

func TestSessionFromEnv_Replay(t *testing.T) {
	dir := t.TempDir()
	// Seed a cassette directly via FileCassette to skip the
	// unexported-field limitation of fakeResp, then prove the
	// env-configured replay session finds and decodes it.
	c := xrr.NewFileCassette(dir)
	reqPayload := map[string]any{"argv": []any{"echo", "hi"}}
	respPayload := map[string]any{"stdout": "hi", "exit_code": 0}
	require.NoError(t, c.Save("exec", "envrep01", reqPayload, respPayload, nil))

	t.Setenv(xrr.EnvMode, string(xrr.ModeReplay))
	t.Setenv(xrr.EnvCassetteDir, dir)

	repSess, err := xrr.SessionFromEnv()
	require.NoError(t, err)
	require.NotNil(t, repSess)

	adapter := &fakeAdapter{id: "exec", fp: "envrep01"}
	req := &fakeReq{}
	resp, err := repSess.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		t.Fatal("do() must not run in replay")
		return nil, nil
	})
	require.NoError(t, err)
	raw := resp.(*xrr.RawResponse)
	assert.Equal(t, "hi", raw.Payload["stdout"])
}

func TestSessionFromEnv_Passthrough(t *testing.T) {
	// Passthrough doesn't need a dir — confirm SessionFromEnv doesn't
	// demand one, and the returned session never touches a cassette.
	t.Setenv(xrr.EnvMode, string(xrr.ModePassthrough))
	t.Setenv(xrr.EnvCassetteDir, "")

	sess, err := xrr.SessionFromEnv()
	require.NoError(t, err)
	require.NotNil(t, sess)

	called := false
	adapter := &fakeAdapter{id: "exec", fp: "envpt001"}
	req := &fakeReq{}
	_, err = sess.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		called = true
		return &fakeResp{out: "ok"}, nil
	})
	require.NoError(t, err)
	assert.True(t, called, "passthrough must invoke do()")
}

func TestSessionFromEnv_InvalidMode(t *testing.T) {
	t.Setenv(xrr.EnvMode, "nonsense")
	t.Setenv(xrr.EnvCassetteDir, t.TempDir())

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.Contains(t, err.Error(), "nonsense")
}

func TestSessionFromEnv_RecordMissingDir(t *testing.T) {
	t.Setenv(xrr.EnvMode, string(xrr.ModeRecord))
	t.Setenv(xrr.EnvCassetteDir, "")

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.Contains(t, err.Error(), xrr.EnvCassetteDir)
}

func TestSessionFromEnv_ReplayMissingDir(t *testing.T) {
	t.Setenv(xrr.EnvMode, string(xrr.ModeReplay))
	t.Setenv(xrr.EnvCassetteDir, "")

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
}

func TestSessionFromEnv_CassetteDirIsFile(t *testing.T) {
	// Point XRR_CASSETTE_DIR at an existing non-directory file in
	// record mode; SessionFromEnv must refuse.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	t.Setenv(xrr.EnvMode, string(xrr.ModeRecord))
	t.Setenv(xrr.EnvCassetteDir, f)

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.Contains(t, err.Error(), "not a directory")
}

// TestSessionFromEnv_RecordCreatesMissingDir — record mode must create
// the cassette dir when it doesn't exist, so first-write doesn't fail
// with a less-helpful OS error.
func TestSessionFromEnv_RecordCreatesMissingDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "does", "not", "exist", "yet")

	t.Setenv(xrr.EnvMode, string(xrr.ModeRecord))
	t.Setenv(xrr.EnvCassetteDir, dir)

	sess, err := xrr.SessionFromEnv()
	require.NoError(t, err)
	require.NotNil(t, sess)

	info, err := os.Stat(dir)
	require.NoError(t, err, "record mode must create XRR_CASSETTE_DIR if it doesn't exist")
	assert.True(t, info.IsDir())

	// And it must actually be usable for a record round-trip.
	adapter := &fakeAdapter{id: "exec", fp: "mkdir001"}
	req := &fakeReq{}
	_, err = sess.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return &fakeResp{out: "hi"}, nil
	})
	require.NoError(t, err)
}

// TestSessionFromEnv_ReplayMissingDirFailsFast — replay with a
// non-existent dir must surface a config-level error at startup, not
// a downstream cassette-miss.
func TestSessionFromEnv_ReplayMissingDirFailsFast(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nope")

	t.Setenv(xrr.EnvMode, string(xrr.ModeReplay))
	t.Setenv(xrr.EnvCassetteDir, dir)

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestSessionFromEnv_ReplayDirIsFile — replay with a file path must
// surface a config-level error, not a cassette-miss.
func TestSessionFromEnv_ReplayDirIsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	t.Setenv(xrr.EnvMode, string(xrr.ModeReplay))
	t.Setenv(xrr.EnvCassetteDir, f)

	sess, err := xrr.SessionFromEnv()
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.Contains(t, err.Error(), "not a directory")
}
