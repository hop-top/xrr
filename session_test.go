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
	require.NoError(t, c.Save("exec", "a3f9c1b2", fakeReqPayload, fakeRespPayload))

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
