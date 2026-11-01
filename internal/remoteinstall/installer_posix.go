package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/mcnull/sshex/internal/sessionfile"
)

// PosixInstaller installs sshex on POSIX hosts. Linux and macOS remotes share
// the same shell scripts; only the uploaded binary differs.
type PosixInstaller struct {
	binaries BinaryResolver
}

func NewPosixInstaller(binaries BinaryResolver) *PosixInstaller {
	return &PosixInstaller{binaries: binaries}
}

func (i *PosixInstaller) Detect(ctx context.Context, conn Runner) (Platform, error) {
	return DetectPlatform(ctx, conn)
}

func (i *PosixInstaller) RemotePaths(ctx context.Context, conn Runner) (RemotePaths, error) {
	var buf bytes.Buffer
	if err := conn.Run(ctx, pathsScript, nil, &buf, nil); err != nil {
		return RemotePaths{}, err
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return RemotePaths{}, fmt.Errorf("could not determine remote paths")
	}
	return RemotePaths{DataDir: lines[0], RunDir: lines[1]}, nil
}

func (i *PosixInstaller) Install(ctx context.Context, conn Runner, paths RemotePaths, platform Platform) error {
	exe, err := i.binaries.Resolve(ctx, platform)
	if err != nil {
		return err
	}
	f, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()

	bin := paths.BinPath()
	script := `umask 077; mkdir -p ` + shellQuote(paths.BinDir()) + ` ` + shellQuote(paths.SessionsDir()) + ` ` + shellQuote(paths.RunDir) + ` "$HOME/.local/bin"; ` +
		`cat > ` + shellQuote(bin+".new") + ` && chmod 700 ` + shellQuote(bin+".new") + ` && mv -f ` + shellQuote(bin+".new") + ` ` + shellQuote(bin) + `; ` +
		`ln -sf ` + shellQuote(bin) + ` "$HOME/.local/bin/sshex"`
	if err := conn.Run(ctx, script, f, nil, nil); err != nil {
		return fmt.Errorf("install remote sshex: %w", err)
	}
	return nil
}

func (i *PosixInstaller) WriteSession(ctx context.Context, conn Runner, paths RemotePaths, file sessionfile.File) error {
	data, err := sessionfile.Marshal(file)
	if err != nil {
		return err
	}
	target := shellQuote(path.Join(paths.SessionsDir(), file.ID+".json"))
	script := `umask 077; mkdir -p ` + shellQuote(paths.SessionsDir()) + `; cat > ` + target + ` && chmod 600 ` + target
	if err := conn.Run(ctx, script, bytes.NewReader(data), nil, nil); err != nil {
		return fmt.Errorf("write remote session file: %w", err)
	}
	return nil
}

func (i *PosixInstaller) Remove(ctx context.Context, conn Runner, paths RemotePaths, file sessionfile.File) error {
	script := `rm -f ` + shellQuote(path.Join(paths.SessionsDir(), file.ID+".json")) + ` ` + shellQuote(path.Join(paths.RunDir, file.ID+".sock"))
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("remove remote session: %w", err)
	}
	return nil
}

func (i *PosixInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error {
	if err := conn.Run(ctx, uninstallScript(paths), nil, nil, nil); err != nil {
		return fmt.Errorf("uninstall remote sshex: %w", err)
	}
	return nil
}

// uninstallScript removes the ~/.local/bin link, the data directory and the
// binary recorded by install.sh. The recorded binary is removed last so a
// permission failure is reported without skipping the rest of the cleanup.
func uninstallScript(paths RemotePaths) string {
	record := shellQuote(paths.InstallDirRecordPath())
	return `rm -f "$HOME/.local/bin/sshex"; ` +
		`rm -rf ` + shellQuote(paths.BinDir()) + ` ` + shellQuote(paths.SessionsDir()) + ` ` + shellQuote(paths.CompletionsDir()) + ` ` + shellQuote(paths.DistDir()) + ` ` + shellQuote(paths.RunDir) + `; ` +
		`status=0; ` +
		`if [ -f ` + record + ` ]; then ` +
		`install_dir=$(cat ` + record + `); ` +
		`if [ -n "$install_dir" ]; then ` +
		`if rm -f "$install_dir/sshex"; then rm -f ` + record + `; else status=1; fi; ` +
		`else rm -f ` + record + `; fi; ` +
		`fi; ` +
		`rmdir ` + shellQuote(paths.DataDir) + ` 2>/dev/null || true; ` +
		`[ "$status" -eq 0 ]`
}

func RemoteSocketPath(runDir, id string) string {
	return path.Join(runDir, id+".sock")
}

func Executable() (string, error) {
	return os.Executable()
}
