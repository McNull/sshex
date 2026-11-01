package remoteinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/assets"
)

type fakeBuilder struct {
	err   error
	calls []Platform
}

func (b *fakeBuilder) Build(_ context.Context, plat Platform, dst string) error {
	b.calls = append(b.calls, plat)
	if b.err != nil {
		return b.err
	}
	return writeBinary(dst, []byte("#!/bin/sh\necho "+assets.Version()+"\n"))
}

type fakeDownloader struct {
	err   error
	calls []Platform
}

func (d *fakeDownloader) Download(_ context.Context, plat Platform, dst string) error {
	d.calls = append(d.calls, plat)
	if d.err != nil {
		return d.err
	}
	return writeBinary(dst, []byte("#!/bin/sh\necho "+assets.Version()+"\n"))
}

// crossPlatform returns a supported platform that differs from the machine
// running the tests, so Resolve takes the cache/builder/downloader path on
// every runner (CI is linux/amd64, developers may be on darwin/arm64).
func crossPlatform(t *testing.T) Platform {
	t.Helper()
	for _, plat := range SupportedPlatforms {
		if !plat.IsLocal() {
			return plat
		}
	}
	t.Fatalf("no supported platform differs from %s/%s", runtime.GOOS, runtime.GOARCH)
	return Platform{}
}

func TestResolveSamePlatformUsesExecutable(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("must not build")}
	store := NewBinaryStore(builder, nil)
	path, err := store.Resolve(context.Background(), Platform{OS: runtime.GOOS, Arch: runtime.GOARCH})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if path == "" || len(builder.calls) != 0 {
		t.Fatalf("Resolve() path = %q, builder calls = %d", path, len(builder.calls))
	}
}

func TestResolveBuildsIntoCache(t *testing.T) {
	t.Setenv("SSHEX_HOME", t.TempDir())
	builder := &fakeBuilder{}
	store := NewBinaryStore(builder, nil)
	plat := crossPlatform(t)
	path, err := store.Resolve(context.Background(), plat)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	want, _ := BinaryCachePath(plat)
	if path != want {
		t.Fatalf("Resolve() = %q, want %q", path, want)
	}
	if len(builder.calls) != 1 || builder.calls[0] != plat {
		t.Fatalf("builder calls = %v", builder.calls)
	}
}

func TestResolveReusesCachedVersion(t *testing.T) {
	t.Setenv("SSHEX_HOME", t.TempDir())
	builder := &fakeBuilder{}
	store := NewBinaryStore(builder, nil)
	plat := crossPlatform(t)
	cached, _ := BinaryCachePath(plat)
	if err := writeBinary(cached, []byte("#!/bin/sh\necho "+assets.Version()+"\n")); err != nil {
		t.Fatal(err)
	}
	path, err := store.Resolve(context.Background(), plat)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if path != cached || len(builder.calls) != 0 {
		t.Fatalf("Resolve() path = %q, builder calls = %d", path, len(builder.calls))
	}
}

func TestResolveRebuildsStaleCache(t *testing.T) {
	t.Setenv("SSHEX_HOME", t.TempDir())
	builder := &fakeBuilder{}
	store := NewBinaryStore(builder, nil)
	plat := crossPlatform(t)
	cached, _ := BinaryCachePath(plat)
	if err := writeBinary(cached, []byte("#!/bin/sh\necho 0.0.0-old\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(context.Background(), plat); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if len(builder.calls) != 1 {
		t.Fatalf("builder calls = %d, want 1", len(builder.calls))
	}
}

func TestResolveFallsBackToDownload(t *testing.T) {
	t.Setenv("SSHEX_HOME", t.TempDir())
	builder := &fakeBuilder{err: ErrNoSource}
	downloader := &fakeDownloader{}
	store := NewBinaryStore(builder, downloader)
	plat := crossPlatform(t)
	if _, err := store.Resolve(context.Background(), plat); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if len(downloader.calls) != 1 || downloader.calls[0] != plat {
		t.Fatalf("downloader calls = %v", downloader.calls)
	}
}

func TestResolveReportsBuilderFailure(t *testing.T) {
	t.Setenv("SSHEX_HOME", t.TempDir())
	store := NewBinaryStore(&fakeBuilder{err: errors.New("no go")}, nil)
	_, err := store.Resolve(context.Background(), crossPlatform(t))
	if err == nil || !strings.Contains(err.Error(), "no go") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestDataHome(t *testing.T) {
	t.Setenv("SSHEX_HOME", "/custom/sshex")
	home, err := DataHome()
	if err != nil {
		t.Fatalf("DataHome() error: %v", err)
	}
	if home != "/custom/sshex" {
		t.Fatalf("DataHome() = %q", home)
	}

	t.Setenv("SSHEX_HOME", "")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	home, err = DataHome()
	if err != nil {
		t.Fatalf("DataHome() error: %v", err)
	}
	if home != "/xdg/data/sshex" {
		t.Fatalf("DataHome() = %q", home)
	}
}

func TestBinaryCachePath(t *testing.T) {
	t.Setenv("SSHEX_HOME", "/data/sshex")
	got, err := BinaryCachePath(Platform{OS: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatalf("BinaryCachePath() error: %v", err)
	}
	if got != "/data/sshex/dist/sshex-darwin-arm64" {
		t.Fatalf("BinaryCachePath() = %q", got)
	}
}

func TestBinaryVersion(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshex")
	if err := writeBinary(exe, []byte("#!/bin/sh\necho 1.2.3\n")); err != nil {
		t.Fatal(err)
	}
	got, err := BinaryVersion(exe)
	if err != nil {
		t.Fatalf("BinaryVersion() error: %v", err)
	}
	if got != "1.2.3" {
		t.Fatalf("BinaryVersion() = %q", got)
	}

	if _, err := BinaryVersion(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("BinaryVersion() of missing file did not error")
	}
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BinaryVersion(filepath.Join(dir, "plain")); err == nil {
		t.Fatal("BinaryVersion() of non-executable file did not error")
	}
}

func TestUnpackZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("sshex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "binary-content"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "dist", "sshex")
	if err := UnpackZip(bytes.NewReader(buf.Bytes()), dst); err != nil {
		t.Fatalf("UnpackZip() error: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary-content" {
		t.Fatalf("UnpackZip() content = %q", data)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("UnpackZip() mode = %v, want executable", info.Mode())
	}
}

func TestUnpackZipWithoutBinary(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("README.txt"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := UnpackZip(bytes.NewReader(buf.Bytes()), filepath.Join(t.TempDir(), "sshex")); err == nil {
		t.Fatal("UnpackZip() error = nil, want error")
	}
}
