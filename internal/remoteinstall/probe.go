package remoteinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/mcnull/sshex/internal/sessionfile"
)

const probeMarker = "<<<SSHEX-SESSION>>>"

type SessionProbe interface {
	Active(ctx context.Context, run Runner, paths RemotePaths) ([]sessionfile.File, error)
}

type LinuxSessionProbe struct{}

func NewLinuxSessionProbe() *LinuxSessionProbe {
	return &LinuxSessionProbe{}
}

func (p *LinuxSessionProbe) Active(ctx context.Context, run Runner, paths RemotePaths) ([]sessionfile.File, error) {
	var buf bytes.Buffer
	if err := run.Run(ctx, sessionProbeScript(paths), nil, &buf, nil); err != nil {
		return nil, err
	}
	return ParseSessionProbe(buf.String()), nil
}

// sessionProbeScript prints each live session file. A session is live when its
// conventional socket exists and, if `ss` is available, is actually listening.
func sessionProbeScript(paths RemotePaths) string {
	sessions := shellQuote(paths.SessionsDir())
	run := shellQuote(paths.RunDir)
	return `sessions=` + sessions + `; run=` + run + `; ` +
		`[ -d "$sessions" ] || exit 0; ` +
		`for f in "$sessions"/*.json; do ` +
		`[ -f "$f" ] || continue; ` +
		`id=${f##*/}; id=${id%.json}; sock="$run/$id.sock"; ` +
		`[ -S "$sock" ] || continue; ` +
		`if command -v ss >/dev/null 2>&1; then ss -xlH 2>/dev/null | grep -Fq -- "$sock" || continue; fi; ` +
		`printf '%s\n' '` + probeMarker + `'; cat "$f"; printf '\n'; ` +
		`done`
}

// ParseSessionProbe parses the marker-delimited JSON blocks produced by
// sessionProbeScript.
func ParseSessionProbe(output string) []sessionfile.File {
	var sessions []sessionfile.File
	for _, block := range strings.Split(output, probeMarker) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var f sessionfile.File
		if err := json.Unmarshal([]byte(block), &f); err != nil {
			continue
		}
		sessions = append(sessions, f)
	}
	return sessions
}
