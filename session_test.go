package xrr_test

import (
	"context"
	"errors"
	"os"
	"testing"

	xrr "hop.top/xrr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdapter implements xrr.Adapter for testing.
type fakeAdapter struct {
	id string
	fp string
}

func (a *fakeAdapter) ID() string { return a.id }
func (a *fakeAdapter) Fingerprint(_ xrr.Request) (string, error) {
	if a.fp != "" {
		return a.fp, nil
	}
	return "testfp01", nil
}
func (a *fakeAdapter) Serialize(v any) ([]byte, error) {
	return []byte("{}"), nil
}
func (a *fakeAdapter) Deserialize(data []byte, target any) error {
	return nil
}

// fakeReq implements xrr.Request.
type fakeReq struct{ key string }

func (r *fakeReq) AdapterID() string { return "exec" }

// fakeResp implements xrr.Response.
type fakeResp struct{ out string }

func (r *fakeResp) AdapterID() string { return "exec" }

var (
	fakeReqPayload  = map[string]any{"argv": []any{"echo", "hello"}}
	fakeRespPayload = map[string]any{"stdout": "hello\n", "exit_code": 0}
)

func TestSessionRecord(t *testing.T) {
	dir := t.TempDir()
	s := xrr.NewSession(xrr.ModeRecord, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec"}
	req := &fakeReq{key: "argv=echo+hello"}
	called := false
	resp, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		called = true
		return &fakeResp{out: "hello\n"}, nil
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "hello\n", resp.(*fakeResp).out)
	// cassette files must exist
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 2) // req + resp
}

func TestSessionReplay(t *testing.T) {
	dir := t.TempDir()
	// seed cassette
	c := xrr.NewFileCassette(dir)
	require.NoError(t, c.Save("exec", "a3f9c1b2", fakeReqPayload, fakeRespPayload, nil))

	s := xrr.NewSession(xrr.ModeReplay, c)
	adapter := &fakeAdapter{id: "exec", fp: "a3f9c1b2"}
	req := &fakeReq{key: "argv=echo+hello"}
	called := false
	_, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		called = true // must NOT be called in replay
		return nil, nil
	})
	require.NoError(t, err)
	assert.False(t, called)
}

func TestSessionReplayMiss(t *testing.T) {
	dir := t.TempDir()
	s := xrr.NewSession(xrr.ModeReplay, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec", fp: "deadbeef"}
	req := &fakeReq{}
	_, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return nil, nil
	})
	require.True(t, errors.Is(err, xrr.ErrCassetteMiss))
}

func TestSessionPassthrough(t *testing.T) {
	dir := t.TempDir()
	s := xrr.NewSession(xrr.ModePassthrough, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec"}
	req := &fakeReq{}
	called := false
	_, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		called = true
		return &fakeResp{out: "ok"}, nil
	})
	require.NoError(t, err)
	assert.True(t, called)
	// no cassette files written
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 0)
}

// TestSessionRecord_PersistsDoError — record mode now writes a cassette
// even when do() fails, and returns the original (resp, err) pair to the
// caller so existing call sites stay unchanged.
func TestSessionRecord_PersistsDoError(t *testing.T) {
	dir := t.TempDir()
	s := xrr.NewSession(xrr.ModeRecord, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec", fp: "errrec01"}
	req := &fakeReq{}
	doErr := errors.New("exit status 1")

	resp, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return &fakeResp{out: "boom"}, doErr
	})

	require.Error(t, err)
	assert.Equal(t, "exit status 1", err.Error())
	assert.Equal(t, "boom", resp.(*fakeResp).out)

	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 2, "req + resp must both be written even on do() error")
}

// TestSessionReplay_ReEmitsRecordedError — replay mode reads the recorded
// error string and returns errors.New(it) alongside the RawResponse.
func TestSessionReplay_ReEmitsRecordedError(t *testing.T) {
	dir := t.TempDir()
	c := xrr.NewFileCassette(dir)
	require.NoError(t, c.Save("exec", "errrep01", fakeReqPayload, fakeRespPayload, errors.New("exit status 2")))

	s := xrr.NewSession(xrr.ModeReplay, c)
	adapter := &fakeAdapter{id: "exec", fp: "errrep01"}
	req := &fakeReq{}

	resp, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		t.Fatal("do() must not run in replay mode")
		return nil, nil
	})

	require.Error(t, err)
	assert.Equal(t, "exit status 2", err.Error())
	require.NotNil(t, resp)
	raw, ok := resp.(*xrr.RawResponse)
	require.True(t, ok, "replay must return *RawResponse alongside the recorded error")
	assert.Equal(t, "hello\n", raw.Payload["stdout"])
}

// TestSessionRecord_NilRespWithError — when do() returns (nil, err) the
// session must still persist a valid v1 cassette (payload: {} on disk,
// not payload: null) and replay must surface RawResponse{Payload: empty}
// + the recorded error string.
func TestSessionRecord_NilRespWithError(t *testing.T) {
	dir := t.TempDir()
	rec := xrr.NewSession(xrr.ModeRecord, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec", fp: "nilres01"}
	req := &fakeReq{}

	resp, err := rec.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	require.Error(t, err)
	assert.Equal(t, "dial tcp: connection refused", err.Error())
	assert.Nil(t, resp, "session passes through nil resp from do()")

	// On-disk resp envelope must have payload: {} (object), never payload: null.
	data, readErr := os.ReadFile(dir + "/exec-nilres01.resp.yaml")
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), "payload: null", "v1 spec requires payload to be a non-null object")
	assert.Contains(t, string(data), "payload: {}", "nil resp must serialize as empty map")

	rep := xrr.NewSession(xrr.ModeReplay, xrr.NewFileCassette(dir))
	replayResp, replayErr := rep.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		t.Fatal("do() must not run in replay mode")
		return nil, nil
	})
	require.Error(t, replayErr)
	assert.Equal(t, "dial tcp: connection refused", replayErr.Error())
	require.NotNil(t, replayResp, "replay must always return RawResponse, even for nil-resp recordings")
	raw, ok := replayResp.(*xrr.RawResponse)
	require.True(t, ok)
	assert.NotNil(t, raw.Payload, "replayed payload must be a non-nil empty map, not nil")
	assert.Empty(t, raw.Payload)
}

// TestSessionReplay_BackwardCompat_NoErrorField — cassettes written by older
// xrr versions (no envelope error field) still replay as success.
func TestSessionReplay_BackwardCompat_NoErrorField(t *testing.T) {
	dir := t.TempDir()
	// Hand-write a v1 cassette with no error field.
	reqYAML := []byte("xrr: \"1\"\nadapter: exec\nfingerprint: oldfmt01\nrecorded_at: \"2026-01-01T00:00:00Z\"\npayload:\n  argv: [\"echo\", \"old\"]\n")
	respYAML := []byte("xrr: \"1\"\nadapter: exec\nfingerprint: oldfmt01\nrecorded_at: \"2026-01-01T00:00:00Z\"\npayload:\n  stdout: \"old\\n\"\n  exit_code: 0\n")
	require.NoError(t, os.WriteFile(dir+"/exec-oldfmt01.req.yaml", reqYAML, 0o644))
	require.NoError(t, os.WriteFile(dir+"/exec-oldfmt01.resp.yaml", respYAML, 0o644))

	s := xrr.NewSession(xrr.ModeReplay, xrr.NewFileCassette(dir))
	adapter := &fakeAdapter{id: "exec", fp: "oldfmt01"}
	req := &fakeReq{}

	resp, err := s.Record(context.Background(), adapter, req, func() (xrr.Response, error) {
		t.Fatal("do() must not run in replay")
		return nil, nil
	})

	require.NoError(t, err, "old recordings without an error field must replay as success")
	require.NotNil(t, resp)
	raw := resp.(*xrr.RawResponse)
	assert.Equal(t, "old\n", raw.Payload["stdout"])
}
