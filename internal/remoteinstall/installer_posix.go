package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
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
	verbosef("probing remote sshex data/runtime paths")
	var buf bytes.Buffer
	if err := conn.Run(ctx, pathsScript, nil, &buf, nil); err != nil {
		return RemotePaths{}, err
	}
	lines := strings.Split(strings.ReplaceAll(buf.String(), "\r\n", "\n"), "\n")
	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case pathsBeginMarker:
			begin = i
		case pathsEndMarker:
			if begin >= 0 && i > begin {
				end = i
			}
		}
	}
	if begin < 0 || end != begin+3 || strings.TrimSpace(lines[begin+1]) == "" || strings.TrimSpace(lines[begin+2]) == "" {
		output := buf.String()
		if len(output) > 256 {
			output = output[:256] + "…"
		}
		return RemotePaths{}, fmt.Errorf("could not determine remote paths from framed probe output: %s", strconv.Quote(output))
	}
	paths := RemotePaths{DataDir: strings.TrimSpace(lines[begin+1]), RunDir: strings.TrimSpace(lines[begin+2])}
	verbosef("remote paths: data=%q runtime=%q", paths.DataDir, paths.RunDir)
	return paths, nil
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

func (i *PosixInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths, opts UninstallOptions) error {
	if err := conn.Run(ctx, uninstallScript(paths, opts), nil, nil, nil); err != nil {
		return fmt.Errorf("uninstall remote sshex: %w", err)
	}
	return nil
}

// uninstallScript removes the ~/.local/bin link, the data directory and the
// binary recorded by install.sh. The recorded binary is removed last so a
// permission failure is reported without skipping the rest of the cleanup.
func uninstallScript(paths RemotePaths, opts UninstallOptions) string {
	record := shellQuote(paths.InstallDirRecordPath())
	script := `rm -f "$HOME/.local/bin/sshex"; ` +
		`rm -rf ` + shellQuote(paths.BinDir()) + ` ` + shellQuote(paths.SessionsDir()) + ` ` + shellQuote(paths.CompletionsDir()) + ` ` + shellQuote(paths.DistDir()) + ` ` + shellQuote(paths.RunDir) + `; ` +
		`status=0; `
	if opts.RemoveConfig {
		// SSHEX_CONFIG lets a local uninstall remove the selected config file;
		// remotes normally fall back to the standard XDG location. For the
		// default location the whole sshex config directory belongs to the
		// installation, including version-check state. An explicitly selected
		// config only removes the file and leaves its directory alone.
		script += `config_file="${SSHEX_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/sshex/config.yml}"; ` +
			`config_dir=$(dirname "$config_file"); ` +
			`default_config="${XDG_CONFIG_HOME:-$HOME/.config}/sshex/config.yml"; ` +
			`if [ "$config_file" = "$default_config" ]; then ` +
			`rm -rf "$config_dir" || status=1; ` +
			`else ` +
			`rm -f "$config_file" || status=1; ` +
			`rmdir "$config_dir" 2>/dev/null || true; ` +
			`fi; `
	}
	script += `if [ -f ` + record + ` ]; then ` +
		`install_dir=$(cat ` + record + `); ` +
		`if [ -n "$install_dir" ]; then ` +
		`if rm -f "$install_dir/sshex"; then rm -f ` + record + `; else status=1; fi; ` +
		`else rm -f ` + record + `; fi; ` +
		`fi; ` +
		`rmdir ` + shellQuote(paths.DataDir) + ` 2>/dev/null || true; ` +
		`[ "$status" -eq 0 ]`
	return script
}

func RemoteSocketPath(runDir, id string) string {
	return path.Join(runDir, id+".sock")
}

func Executable() (string, error) {
	return os.Executable()
}
