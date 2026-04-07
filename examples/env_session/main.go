// Package main shows the canonical cross-process e2e adoption pattern
// for xrr: call xrr.SessionFromEnv() in your binary's main() and wire
// the returned session into your runners. The parent test sets
// XRR_MODE + XRR_CASSETTE_DIR in the child environment before exec'ing
// this binary.
//
// Usage:
//
//	# record
//	XRR_MODE=record XRR_CASSETTE_DIR=/tmp/xrr go run ./examples/env_session
//
//	# replay — real subprocess is NOT invoked
//	XRR_MODE=replay XRR_CASSETTE_DIR=/tmp/xrr go run ./examples/env_session
//
//	# no env — passthrough / fall through to normal execution
//	go run ./examples/env_session
package main

import (
	"context"
	"fmt"
	"log"
	osexec "os/exec"

	xrr "hop.top/xrr"
	xexec "hop.top/xrr/adapters/exec"
)

func main() {
	sess, err := xrr.SessionFromEnv()
	if err != nil {
		log.Fatalf("xrr env: %v", err)
	}

	adapter := xexec.NewAdapter()
	req := &xexec.Request{Argv: []string{"echo", "hello from xrr env"}}

	// runEcho is the "real" side that only gets invoked in record
	// mode. In replay mode do() is never called and we serve from
	// the cassette dir the parent controls.
	runEcho := func() (xrr.Response, error) {
		out, runErr := osexec.Command(req.Argv[0], req.Argv[1:]...).Output()
		return &xexec.Response{
			Stdout:   string(out),
			ExitCode: xexec.ExitCodeFromError(runErr),
		}, runErr
	}

	if sess == nil {
		// XRR_MODE unset — fall back to plain execution. Production
		// code paths that don't run under a test harness land here.
		fmt.Println("(no xrr session; running real subprocess)")
		resp, err := runEcho()
		if err != nil {
			log.Fatalf("real: %v", err)
		}
		fmt.Printf("real: %s", resp.(*xexec.Response).Stdout)
		return
	}

	resp, err := sess.Record(context.Background(), adapter, req, runEcho)
	if err != nil {
		log.Fatalf("xrr session: %v", err)
	}

	// resp shape differs by mode: *xexec.Response in record mode,
	// *xrr.RawResponse in replay mode. Both paths should look the
	// same to the caller.
	switch r := resp.(type) {
	case *xexec.Response:
		fmt.Printf("recorded: %s", r.Stdout)
	case *xrr.RawResponse:
		fmt.Printf("replayed: %s", r.Payload["stdout"])
	}
}
