package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/sshx"
)

const portProbeTimeout = 5 * time.Second

// PortListHost selects which machine a listening-port probe runs on.
type PortListHost string

const (
	PortListLocal  PortListHost = "local"
	PortListRemote PortListHost = "remote"
)

// ListeningPort is one TCP port that a process is listening on.
type ListeningPort struct {
	Port    int
	Address string
	Process string
	PID     int
}

// CommandRunner runs one shell command on the local machine or over an SSH
// connection. Both local.Runner and sshx.Connection satisfy it.
type CommandRunner interface {
	Run(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error
}

// PortProbe turns a command runner into a list of listening ports.
type PortProbe interface {
	Probe(ctx context.Context, run CommandRunner) ([]ListeningPort, error)
}

// ListeningServiceConfig wires the state and runners needed to list ports.
type ListeningServiceConfig struct {
	Sessions SessionStore
	Pool     *sshx.Pool
	Local    CommandRunner
	Probe    PortProbe
}

// ListeningService lists TCP ports with the process that owns them. Local
// probes run on the origin; remote probes reuse the session's control master.
type ListeningService struct {
	sessions SessionStore
	pool     *sshx.Pool
	local    CommandRunner
	probe    PortProbe
}

func NewListeningService(cfg ListeningServiceConfig) *ListeningService {
	probe := cfg.Probe
	if probe == nil {
		probe = NewPosixPortProbe()
	}
	return &ListeningService{
		sessions: cfg.Sessions,
		pool:     cfg.Pool,
		local:    cfg.Local,
		probe:    probe,
	}
}

// List returns the listening TCP ports of one side of a session.
func (s *ListeningService) List(ctx context.Context, sessionID string, host PortListHost) ([]ListeningPort, error) {
	if host != PortListLocal && host != PortListRemote {
		return nil, fmt.Errorf("%w: invalid port host %q", errs.ErrInvalidInput, host)
	}
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return nil, err
	}

	runner := s.local
	if host == PortListRemote {
		conn, ok := s.pool.Controller(sessionID)
		if !ok {
			return nil, errs.ErrNoSession
		}
		runner = conn
	}
	if runner == nil {
		return nil, fmt.Errorf("%w: no runner for host %q", errs.ErrPortListUnavailable, host)
	}

	probeCtx, cancel := context.WithTimeout(ctx, portProbeTimeout)
	defer cancel()
	return s.probe.Probe(probeCtx, runner)
}

// NewPosixPortProbe returns a probe that prefers ss, falls back to lsof and
// finally netstat. It works on Linux and macOS, locally and over SSH.
func NewPosixPortProbe() PortProbe { return posixPortProbe{} }

type posixPortProbe struct{}

func (posixPortProbe) Probe(ctx context.Context, run CommandRunner) ([]ListeningPort, error) {
	var stdout, stderr bytes.Buffer
	if err := run.Run(ctx, portProbeScript, nil, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("list listening ports: %w", err)
	}
	return parsePortProbeOutput(stdout.String())
}

// portProbeScript chooses the first available tool and tags its output so the
// parser does not have to guess. Each tool ends with `|| true` because lsof
// exits non-zero when nothing matches.
const portProbeScript = `if command -v ss >/dev/null 2>&1; then
  echo '@@ss'
  ss -H -tlnp 2>/dev/null || true
elif command -v lsof >/dev/null 2>&1; then
  echo '@@lsof'
  lsof -nP -iTCP -sTCP:LISTEN -FpcPn 2>/dev/null || true
elif command -v netstat >/dev/null 2>&1; then
  echo '@@netstat'
  case "$(uname -s 2>/dev/null)" in
    Darwin) netstat -an -p tcp 2>/dev/null || true ;;
    *) netstat -tlnp 2>/dev/null || true ;;
  esac
else
  echo '@@none'
fi`

// parsePortProbeOutput parses the tagged output of portProbeScript.
func parsePortProbeOutput(output string) ([]ListeningPort, error) {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		switch line {
		case "@@ss":
			return normalizeListeningPorts(parseSSPorts(lines[i+1:])), nil
		case "@@lsof":
			return normalizeListeningPorts(parseLsofPorts(lines[i+1:])), nil
		case "@@netstat":
			return normalizeListeningPorts(parseNetstatPorts(lines[i+1:])), nil
		case "@@none":
			return nil, fmt.Errorf("%w: ss, lsof and netstat are missing", errs.ErrPortListUnavailable)
		}
	}
	return nil, fmt.Errorf("%w: unrecognized probe output", errs.ErrPortListUnavailable)
}

func parseSSPorts(lines []string) []ListeningPort {
	var ports []ListeningPort
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "LISTEN") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		address, port, ok := parseHostPort(fields[3], ':')
		if !ok {
			continue
		}
		process, pid := parseSSProcess(line)
		ports = append(ports, ListeningPort{Port: port, Address: address, Process: process, PID: pid})
	}
	return ports
}

func parseSSProcess(line string) (string, int) {
	start := strings.Index(line, "((\"")
	if start < 0 {
		return "", 0
	}
	rest := line[start+3:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return "", 0
	}
	process := rest[:end]
	pidStart := strings.Index(rest[end:], "pid=")
	if pidStart < 0 {
		return process, 0
	}
	rest = rest[end+pidStart+4:]
	pidEnd := 0
	for pidEnd < len(rest) && rest[pidEnd] >= '0' && rest[pidEnd] <= '9' {
		pidEnd++
	}
	pid, _ := strconv.Atoi(rest[:pidEnd])
	return process, pid
}

func parseLsofPorts(lines []string) []ListeningPort {
	var ports []ListeningPort
	var pid int
	process := ""
	for _, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
			process = ""
		case 'c':
			process = line[1:]
		case 'n':
			address, port, ok := parseHostPort(line[1:], ':')
			if !ok {
				continue
			}
			ports = append(ports, ListeningPort{Port: port, Address: address, Process: process, PID: pid})
		}
	}
	return ports
}

func parseNetstatPorts(lines []string) []ListeningPort {
	var ports []ListeningPort
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.HasPrefix(line, "tcp") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		stateAt := -1
		for i, field := range fields {
			if field == "LISTEN" {
				stateAt = i
				break
			}
		}
		if stateAt < 0 {
			continue
		}
		address, port, ok := parseHostPort(fields[3], ':')
		if !ok {
			address, port, ok = parseHostPort(fields[3], '.')
		}
		if !ok {
			continue
		}
		process, pid := "", 0
		if stateAt+1 < len(fields) {
			process, pid = parseNetstatProcess(fields[stateAt+1])
		}
		ports = append(ports, ListeningPort{Port: port, Address: address, Process: process, PID: pid})
	}
	return ports
}

func parseNetstatProcess(value string) (string, int) {
	if value == "" || value == "-" {
		return "", 0
	}
	pidText, process, found := strings.Cut(value, "/")
	if !found {
		return "", 0
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return "", 0
	}
	return process, pid
}

// parseHostPort splits a listening address at the last separator and returns
// the address, the numeric port and whether the value looked valid.
func parseHostPort(value string, sep byte) (string, int, bool) {
	value = strings.TrimSpace(value)
	at := strings.LastIndexByte(value, sep)
	if at < 0 || at == len(value)-1 {
		return "", 0, false
	}
	port, err := strconv.Atoi(value[at+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	host := strings.TrimSpace(value[:at])
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if host == "" {
		host = "*"
	}
	return host, port, true
}

func normalizeListeningPorts(items []ListeningPort) []ListeningPort {
	out := make([]ListeningPort, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Port < 1 || item.Port > 65535 {
			continue
		}
		key := fmt.Sprintf("%d|%d|%s", item.Port, item.PID, item.Process)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		left, right := strings.ToLower(out[i].Process), strings.ToLower(out[j].Process)
		if left != right {
			if left == "" {
				return false
			}
			if right == "" {
				return true
			}
			return left < right
		}
		return out[i].PID < out[j].PID
	})
	return out
}
