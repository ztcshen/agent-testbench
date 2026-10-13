package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// The desktop holds this pipe open for its owned execution engine. EOF also
// arrives when its parent is killed before the ordinary quit hooks can run.
func runOwnedServe(addr string, handler http.Handler, owner io.Reader) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return serveWhileOwnerConnected(listener, handler, owner)
}

func serveWhileOwnerConnected(listener net.Listener, handler http.Handler, owner io.Reader) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_, _ = io.Copy(io.Discard, owner)
		_ = server.Close()
	}()
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
