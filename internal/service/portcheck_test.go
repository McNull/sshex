package service

import (
	"net"
	"testing"
)

func TestLocalPortInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	inUse, err := localPortInUse(port)
	if err != nil {
		t.Fatalf("localPortInUse(%d) error: %v", port, err)
	}
	if !inUse {
		t.Fatalf("localPortInUse(%d) = false, want true", port)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	inUse, err = localPortInUse(port)
	if err != nil {
		t.Fatalf("localPortInUse(%d) after close error: %v", port, err)
	}
	if inUse {
		t.Fatalf("localPortInUse(%d) = true after close, want false", port)
	}
}
