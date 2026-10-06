package remoteinstall

import (
	"context"
	"strings"
	"testing"
)

func TestParseSessionProbe(t *testing.T) {
	output := probeMarker + "\n" +
		`{"id":"abc","origin":"laptop","endpoint":{"kind":"unix","address":"/run/abc.sock"},"token":"t","capability":"tunnels","target":"bob@ssh02"}` + "\n" +
		probeMarker + "\n" +
		`{"id":"def","endpoint":{"kind":"unix","address":"/run/def.sock"},"token":"u","capability":"tunnels"}` + "\n"

	got := ParseSessionProbe(output)
	if len(got) != 2 {
		t.Fatalf("ParseSessionProbe() returned %d sessions, want 2", len(got))
	}
	if got[0].ID != "abc" || got[0].Origin != "laptop" || got[0].Target != "bob@ssh02" {
		t.Fatalf("first session = %+v", got[0])
	}
	if got[1].ID != "def" {
		t.Fatalf("second session = %+v", got[1])
	}
}

func TestParseSessionProbeIgnoresGarbage(t *testing.T) {
	got := ParseSessionProbe("noise\n" + probeMarker + "\nnot json\n")
	if len(got) != 0 {
		t.Fatalf("ParseSessionProbe() = %+v, want none", got)
	}
}

func TestSessionProbeScript(t *testing.T) {
	script := sessionProbeScript(RemotePaths{DataDir: "/data", RunDir: "/run"})
	for _, want := range []string{"/data/sessions", "/run", ".sock", "ss -xlH", probeMarker} {
		if !strings.Contains(script, want) {
			t.Fatalf("sessionProbeScript() missing %q:\n%s", want, script)
		}
	}
}

func TestLinuxSessionProbeRunsScript(t *testing.T) {
	conn := &fakeConn{}
	p := NewLinuxSessionProbe()
	sessions, err := p.Active(context.Background(), conn, RemotePaths{DataDir: "/data", RunDir: "/run"})
	if err != nil {
		t.Fatalf("Active() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("Active() = %+v, want none", sessions)
	}
	if len(conn.commands) != 1 {
		t.Fatalf("Active() ran %d commands, want 1", len(conn.commands))
	}
}
