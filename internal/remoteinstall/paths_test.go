package remoteinstall

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/sessionfile"
)

func TestPathsScriptMatchesSessionfileRemoteDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pathsScript requires a POSIX shell")
	}

	cases := []struct {
		name      string
		home      string
		dataHome  string
		sshexHome string
	}{
		{name: "default", home: "/home/tester"},
		{name: "xdg data", home: "/home/tester", dataHome: "/custom/data"},
		{name: "sshex home", home: "/home/tester", dataHome: "/custom/data", sshexHome: "/custom/sshex"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			t.Setenv("XDG_DATA_HOME", tc.dataHome)
			t.Setenv("SSHEX_HOME", tc.sshexHome)
			t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")

			out, err := exec.Command("sh", "-c", pathsScript).Output()
			if err != nil {
				t.Fatalf("pathsScript error: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) != 2 {
				t.Fatalf("pathsScript output = %q, want two lines", out)
			}

			sessionsDir, err := sessionfile.RemoteDir()
			if err != nil {
				t.Fatalf("RemoteDir() error: %v", err)
			}
			if want := filepath.Dir(sessionsDir); lines[0] != want {
				t.Fatalf("data dir = %q, want %q (dir of RemoteDir)", lines[0], want)
			}
			if want := "/run/user/501/sshex"; lines[1] != want {
				t.Fatalf("runtime dir = %q, want %q", lines[1], want)
			}
		})
	}
}
