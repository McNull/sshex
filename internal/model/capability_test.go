package model

import "testing"

func TestCapabilityValid(t *testing.T) {
	tests := []struct {
		name string
		cap  Capability
		want bool
	}{
		{name: "tunnels", cap: CapabilityTunnels, want: true},
		{name: "exec", cap: CapabilityExec, want: true},
		{name: "commands", cap: CapabilityCommands, want: true},
		{name: "empty", cap: Capability(""), want: false},
		{name: "unknown", cap: Capability("admin"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Fatalf("Capability(%q).Valid() = %v, want %v", tt.cap, got, tt.want)
			}
		})
	}
}
