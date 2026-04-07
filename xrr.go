package xrr

import (
	"context"
	"errors"
)

// Mode controls session behaviour.
type Mode string

const (
	ModeRecord      Mode = "record"
	ModeReplay      Mode = "replay"
	ModePassthrough Mode = "passthrough"
)

// ErrCassetteMiss is returned when replay finds no matching cassette file.
var ErrCassetteMiss = errors.New("xrr: cassette miss")

// Adapter intercepts one channel type.
type Adapter interface {
	ID() string
	Fingerprint(req Request) (string, error)
	Serialize(v any) ([]byte, error)
	Deserialize(data []byte, target any) error
}

// Request is an opaque adapter-defined value.
type Request interface {
	AdapterID() string
}

// Response is an opaque adapter-defined value.
type Response interface {
	AdapterID() string
}

// Cassette reads/writes interaction files.
//
// Save persists a recorded interaction. recordedErr is the error returned by
// the original do() (or nil); when non-nil it is serialized as the envelope
// error field on the resp file so replay can re-emit a matching error.
//
// Load returns recordedErr as a string from the resp envelope's optional
// error field. Empty string ⇒ success; non-empty ⇒ replay should surface
// errors.New(recordedErr) alongside respTarget. ErrCassetteMiss is returned
// if no matching files exist.
type Cassette interface {
	Load(adapterID, fingerprint string, reqTarget, respTarget any) (recordedErr string, err error)
	Save(adapterID, fingerprint string, req, resp any, recordedErr error) error
}

// Session owns the lifecycle of one record/replay run.
type Session interface {
	Record(ctx context.Context, adapter Adapter, req Request,
		do func() (Response, error)) (Response, error)
	Close() error
}
