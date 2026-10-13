package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOwnedServeStopsWhenParentPipeCloses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		done <- serveWhileOwnerConnected(listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), reader)
	}()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("live engine status = %d", response.StatusCode)
	}
	writer.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owned engine survived its parent pipe")
	}
	if response, err := client.Get("http://" + listener.Addr().String()); err == nil {
		response.Body.Close()
		t.Fatal("retired engine still accepts requests")
	}
}
