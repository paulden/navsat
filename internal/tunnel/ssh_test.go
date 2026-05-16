package tunnel

import (
	"context"
	"net"
	"testing"
)

func TestWaitReadySuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port
	tun := &Tunnel{Port: port}

	if err := tun.waitReady(context.Background()); err != nil {
		t.Errorf("waitReady with active listener: %v", err)
	}
}

func TestWaitReadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tun := &Tunnel{Port: 59999} // nothing listening on this port
	err := tun.waitReady(ctx)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
