package service

import "testing"

func TestParsePortSpec(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		wantLocal  int
		wantHost   string
		wantTarget int
		wantErr    bool
	}{
		{name: "port only", spec: "8080", wantLocal: 8080, wantHost: "localhost", wantTarget: 8080},
		{name: "host only", spec: "8080:server01", wantLocal: 8080, wantHost: "server01", wantTarget: 8080},
		{name: "host and port", spec: "8080:server01:80", wantLocal: 8080, wantHost: "server01", wantTarget: 80},
		{name: "localhost", spec: "8080:localhost:80", wantLocal: 8080, wantHost: "localhost", wantTarget: 80},
		{name: "empty", spec: "", wantErr: true},
		{name: "leading colon", spec: ":8080", wantErr: true},
		{name: "trailing colon", spec: "1:", wantLocal: 1, wantHost: "localhost", wantTarget: 1},
		{name: "empty host", spec: "8080::80", wantLocal: 8080, wantHost: "localhost", wantTarget: 80},
		{name: "leading zeros", spec: "08080", wantLocal: 8080, wantHost: "localhost", wantTarget: 8080},
		{name: "bad local port", spec: "0", wantErr: true},
		{name: "non numeric", spec: "abc", wantErr: true},
		{name: "bad target port", spec: "8080:server01:99999", wantErr: true},
		{name: "too many parts", spec: "1:2:3:4", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePortSpec(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePortSpec(%q) expected error, got %+v", tt.spec, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePortSpec(%q) unexpected error: %v", tt.spec, err)
			}
			if got.LocalPort != tt.wantLocal || got.TargetHost != tt.wantHost || got.TargetPort != tt.wantTarget {
				t.Fatalf("ParsePortSpec(%q) = %+v, want local=%d host=%s target=%d",
					tt.spec, got, tt.wantLocal, tt.wantHost, tt.wantTarget)
			}
		})
	}
}
