package remoteinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mcnull/sshex/internal/assets"
)

// ErrNoSource reports that sshex was built without a Go source tree available.
var ErrNoSource = errors.New("no sshex source tree found")

// BinaryResolver produces the local path of an sshex binary suitable for the
// remote platform. The returned path is streamed to the remote host during
// install.
type BinaryResolver interface {
	Resolve(ctx context.Context, plat Platform) (string, error)
}

// Builder cross-compiles sshex for a platform into dst.
type Builder interface {
	Build(ctx context.Context, plat Platform, dst string) error
}

// Downloader places a prebuilt sshex binary for a platform into dst.
type Downloader interface {
	Download(ctx context.Context, plat Platform, dst string) error
}

// BinaryStore resolves platform-matched sshex binaries. Same-platform requests
// reuse the running executable; cross-platform requests use a cached
// cross-compiled binary under the sshex data directory, rebuilding it when its
// version no longer matches, and falling back to a downloaded release build.
type BinaryStore struct {
	builder    Builder
	downloader Downloader
}

func NewBinaryStore(builder Builder, downloader Downloader) *BinaryStore {
	return &BinaryStore{builder: builder, downloader: downloader}
}

func (s *BinaryStore) Resolve(ctx context.Context, plat Platform) (string, error) {
	if plat.IsLocal() {
		return os.Executable()
	}
	dst, err := BinaryCachePath(plat)
	if err != nil {
		return "", err
	}
	if version, err := BinaryVersion(dst); err == nil && version == assets.Version() {
		return dst, nil
	}
	var cause error
	if s.builder != nil {
		if cause = s.builder.Build(ctx, plat, dst); cause == nil {
			return dst, nil
		}
	}
	if s.downloader != nil {
		if err := s.downloader.Download(ctx, plat, dst); err == nil {
			return dst, nil
		} else if cause == nil {
			cause = err
		}
	}
	if cause == nil {
		cause = fmt.Errorf("no builder or downloader for %s", plat)
	}
	return "", fmt.Errorf("obtain sshex binary for %s: %w", plat, cause)
}

// DataHome returns the local sshex data root, mirroring the layout used on
// remote hosts: SSHEX_HOME wins, then XDG_DATA_HOME, then ~/.local/share.
func DataHome() (string, error) {
	if root := os.Getenv("SSHEX_HOME"); root != "" {
		return root, nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "sshex"), nil
}

// BinaryCachePath returns where the cross-compiled binary for a platform is
// cached: <dataHome>/dist/sshex-<os>-<arch>.
func BinaryCachePath(plat Platform) (string, error) {
	root, err := DataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "dist", "sshex-"+plat.OS+"-"+plat.Arch), nil
}

// BinaryVersion reports the version a cached binary prints; it errors when the
// binary is missing or unusable so the caller knows to refresh the cache.
func BinaryVersion(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&0o100 == 0 {
		return "", fmt.Errorf("%s is not executable", path)
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GoBuilder cross-compiles sshex from a Go source tree. Source lookup walks up
// from the process working directory and the executing binary's directory, so
// installed release binaries without a checkout skip straight to downloading.
type GoBuilder struct{}

func NewGoBuilder() *GoBuilder { return &GoBuilder{} }

func (b *GoBuilder) Build(ctx context.Context, plat Platform, dst string) error {
	dir, err := moduleDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-tags", "osusergo,netgo", "-ldflags", "-s -w", "-o", dst, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOOS="+plat.OS,
		"GOARCH="+plat.Arch,
		"CGO_ENABLED=0",
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build sshex for %s: %w", plat, err)
	}
	return os.Chmod(dst, 0o700)
}

func moduleDir() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if dir, ok := moduleDirFrom(start); ok {
			return dir, nil
		}
	}
	return "", ErrNoSource
}

func moduleDirFrom(start string) (string, bool) {
	dir := start
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// ReleaseDownloader fetches the prebuilt sshex binary for a platform from the
// latest GitHub release assets.
type ReleaseDownloader struct {
	repo string
}

func NewReleaseDownloader() *ReleaseDownloader { return &ReleaseDownloader{repo: "McNull/sshex"} }

func (d *ReleaseDownloader) Download(ctx context.Context, plat Platform, dst string) error {
	asset := "sshex-" + plat.OS + "-" + plat.Arch + ".zip"
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", d.repo, asset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return UnpackZip(resp.Body, dst)
}

// UnpackZip extracts the sshex binary from a zip archive read from r and writes
// it to dst with executable permissions.
func UnpackZip(r io.Reader, dst string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range archive.File {
		if f.FileInfo().IsDir() || filepath.Base(f.Name) != "sshex" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		return writeBinary(dst, buf.Bytes())
	}
	return errors.New("zip archive did not contain an sshex binary")
}

func writeBinary(dst string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, content, 0o700); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
