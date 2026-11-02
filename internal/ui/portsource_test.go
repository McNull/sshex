package ui

import (
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
)

func TestPortFromResponse(t *testing.T) {
	got := portFromResponse(api.ListeningPortResponse{
		Port:    8080,
		Address: "127.0.0.1",
		Process: "node",
		PID:     321,
	})
	want := tunnels.Port{Port: 8080, Address: "127.0.0.1", Process: "node", PID: 321}
	if got != want {
		t.Fatalf("portFromResponse() = %+v, want %+v", got, want)
	}
}
