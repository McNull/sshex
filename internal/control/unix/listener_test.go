package unix

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestServeShutsDownGracefullyAfterResponse reproduces a handler that asks to
// shut the broker down while it is still serving a response, as session close
// does. The response must be delivered before the connection is closed.
func TestServeShutsDownGracefullyAfterResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unix socket paths are limited to about 100 bytes, so keep the temporary
	// directory short rather than using the long per-test temp dir.
	dir, err := os.MkdirTemp("/tmp", "sshex-listener")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	provider := NewProvider(dir)
	listener, err := provider.Listen(ctx, "daemon")
	if err != nil {
		t.Fatalf("Listen() error: %v", err)
	}

	const payload = "dropped tunnels"
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
		cancel()
		// Hold the handler open long enough for a forcible server close to
		// race with writing the response.
		time.Sleep(100 * time.Millisecond)
	})

	serveErr := make(chan error, 1)
	go func() { serveErr <- listener.Serve(ctx, handler) }()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", listener.Endpoint().Address)
		},
	}
	client := &http.Client{Transport: transport}
	resp, err := client.Get("http://sshex/")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("body = %q, want %q", body, payload)
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return after shutdown")
	}
}
