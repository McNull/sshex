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

type LinuxInstaller struct{}

func NewLinuxInstaller() *LinuxInstaller {
	return &LinuxInstaller{}
}

func (i *LinuxInstaller) RemotePaths(ctx context.Context, conn Runner) (RemotePaths, error) {
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

func (i *LinuxInstaller) Install(ctx context.Context, conn Runner, paths RemotePaths) error {
	exe, err := os.Executable()
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

func (i *LinuxInstaller) WriteSession(ctx context.Context, conn Runner, paths RemotePaths, file sessionfile.File) error {
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

func (i *LinuxInstaller) Remove(ctx context.Context, conn Runner, paths RemotePaths, file sessionfile.File) error {
	script := `rm -f ` + shellQuote(path.Join(paths.SessionsDir(), file.ID+".json")) + ` ` + shellQuote(path.Join(paths.RunDir, file.ID+".sock"))
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("remove remote session: %w", err)
	}
	return nil
}

func (i *LinuxInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error {
	script := `rm -f "$HOME/.local/bin/sshex"; ` +
		`rm -rf ` + shellQuote(paths.BinDir()) + ` ` + shellQuote(paths.SessionsDir()) + ` ` + shellQuote(paths.CompletionsDir()) + ` ` + shellQuote(paths.RunDir) + `; ` +
		`rmdir ` + shellQuote(paths.DataDir) + ` 2>/dev/null || true`
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("uninstall remote sshex: %w", err)
	}
	return nil
}

func RemoteSocketPath(runDir, id string) string {
	return path.Join(runDir, id+".sock")
}

func Executable() (string, error) {
	return os.Executable()
}
