package updater

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateReplacesBinaryFromLatestRelease(t *testing.T) {
	binary := []byte("new sshex binary")
	archive := zipBinary(t, binary)
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprint(w, `{"tag_name":"v1.2.0"}`)
		case "/releases/v1.2.0/checksums.txt":
			fmt.Fprintf(w, "%s  sshex-linux-amd64.zip\n", hex.EncodeToString(sum[:]))
		case "/releases/v1.2.0/sshex-linux-amd64.zip":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	executable := filepath.Join(t.TempDir(), "sshex")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := New(Config{
		Client:      server.Client(),
		APIBase:     server.URL + "/latest",
		ReleaseBase: server.URL + "/releases",
		OS:          "linux",
		Arch:        "amd64",
	}).Update(context.Background(), "1.1.0", executable)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.CurrentVersion != "1.1.0" || result.LatestVersion != "1.2.0" {
		t.Fatalf("Update() = %+v", result)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binary) {
		t.Fatalf("installed binary = %q, want %q", got, binary)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode = %o, want 755", info.Mode().Perm())
	}
}

func TestUpdateAlreadyCurrentDoesNotDownloadAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"tag_name":"v1.2.0"}`)
	}))
	defer server.Close()

	result, err := New(Config{
		Client:      server.Client(),
		APIBase:     server.URL + "/latest",
		ReleaseBase: server.URL + "/releases",
		OS:          "linux",
		Arch:        "amd64",
	}).Update(context.Background(), "v1.2.0", filepath.Join(t.TempDir(), "not-needed"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.LatestVersion != "1.2.0" {
		t.Fatalf("Update() = %+v, want already current", result)
	}
}

func TestUpdateRejectsBadChecksumWithoutChangingBinary(t *testing.T) {
	archive := zipBinary(t, []byte("new binary"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprint(w, `{"tag_name":"v1.2.0"}`)
		case "/releases/v1.2.0/checksums.txt":
			fmt.Fprintln(w, strings.Repeat("0", 64)+"  sshex-linux-amd64.zip")
		case "/releases/v1.2.0/sshex-linux-amd64.zip":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	executable := filepath.Join(t.TempDir(), "sshex")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := New(Config{
		Client:      server.Client(),
		APIBase:     server.URL + "/latest",
		ReleaseBase: server.URL + "/releases",
		OS:          "linux",
		Arch:        "amd64",
	}).Update(context.Background(), "1.1.0", executable)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Update() error = %v, want checksum mismatch", err)
	}
	got, readErr := os.ReadFile(executable)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old binary" {
		t.Fatalf("binary changed after failed verification: %q", got)
	}
}

func TestUpdateRejectsUnsupportedPlatform(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v1.2.0"}`)
	}))
	defer server.Close()
	_, err := New(Config{
		Client:  server.Client(),
		APIBase: server.URL,
		OS:      "windows",
		Arch:    "amd64",
	}).Update(context.Background(), "1.1.0", "unused")
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Update() error = %v, want unsupported platform", err)
	}
}

func zipBinary(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf strings.Builder
	w := zip.NewWriter(&buf)
	file, err := w.Create("sshex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}
