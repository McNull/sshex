package remoteinstall

import (
	"context"
	"strings"
	"testing"
)

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    Platform
		wantErr bool
	}{
		{name: "linux amd64", output: "Linux\nx86_64\n", want: Platform{OS: "linux", Arch: "amd64"}},
		{name: "linux arm64", output: "Linux\naarch64\n", want: Platform{OS: "linux", Arch: "arm64"}},
		{name: "darwin arm64", output: "Darwin\narm64\n", want: Platform{OS: "darwin", Arch: "arm64"}},
		{name: "darwin amd64", output: "Darwin\nx86_64\n", want: Platform{OS: "darwin", Arch: "amd64"}},
		{name: "unsupported os", output: "FreeBSD\namd64\n", wantErr: true},
		{name: "unsupported arch", output: "Linux\nriscv64\n", wantErr: true},
		{name: "missing line", output: "Linux\n", wantErr: true},
		{name: "garbage", output: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePlatform(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePlatform(%q) = %+v, want error", tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePlatform(%q) error: %v", tc.output, err)
			}
			if got != tc.want {
				t.Fatalf("ParsePlatform(%q) = %+v, want %+v", tc.output, got, tc.want)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	conn := &fakeConn{stdout: map[string]string{platformScript: "startup warning\n" + platformBeginMarker + "\nLinux\nx86_64\n" + platformEndMarker + "\n"}}
	got, err := DetectPlatform(context.Background(), conn)
	if err != nil {
		t.Fatalf("DetectPlatform() error: %v", err)
	}
	if got != (Platform{OS: "linux", Arch: "amd64"}) {
		t.Fatalf("DetectPlatform() = %+v", got)
	}
}

func TestParsePlatformIgnoresUnframedStartupOutputWhenMarkersPresent(t *testing.T) {
	got, err := ParsePlatform("login banner\n" + platformBeginMarker + "\nLinux\naarch64\n" + platformEndMarker + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Platform{OS: "linux", Arch: "arm64"}) {
		t.Fatalf("platform = %+v", got)
	}
}

func TestPlatformString(t *testing.T) {
	if got := (Platform{OS: "darwin", Arch: "arm64"}).String(); got != "darwin/arm64" {
		t.Fatalf("String() = %q", got)
	}
}

func TestUnsupportedPlatformErrorMentionsSupported(t *testing.T) {
	_, err := ParsePlatform("FreeBSD\namd64\n")
	if err == nil || !strings.Contains(err.Error(), "linux/amd64") {
		t.Fatalf("error = %v, want supported list", err)
	}
}
