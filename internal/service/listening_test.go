package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/repository/memory"
	"github.com/mcnull/sshex/internal/sshx"
)

type stubRunner struct {
	output  string
	err     error
	calls   int
	command string
}

func (s *stubRunner) Run(_ context.Context, command string, _ io.Reader, stdout, _ io.Writer) error {
	s.calls++
	s.command = command
	if s.output != "" && stdout != nil {
		_, _ = io.WriteString(stdout, s.output)
	}
	return s.err
}

const ssFixture = `@@ss
LISTEN 0      4096         0.0.0.0:22            0.0.0.0:*    users:(("sshd",pid=1,fd=3))
LISTEN 0      4096       127.0.0.1:8080          0.0.0.0:*    users:(("node",pid=321,fd=20))
LISTEN 0      4096            [::]:22               [::]:*    users:(("sshd",pid=1,fd=4))
ESTAB   0      0       127.0.0.1:8080     127.0.0.1:50000
`

const lsofFixture = `@@lsof
p1
csshd
f9
PTCP
n*:22
f11
PTCP
n*:22
p99
cpython
f7
PTCP
n127.0.0.1:5432
p321
cnode
f31
PTCP
n*:8080
`

const netstatLinuxFixture = `@@netstat
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 127.0.0.1:631           0.0.0.0:*               LISTEN      123/cupsd
tcp6       0      0 :::22                   :::*                    LISTEN      1/sshd
`

const netstatDarwinFixture = `@@netstat
Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
tcp4       0      0  127.0.0.1.631          *.*                    LISTEN
tcp6       0      0  ::1.8080               *.*                    LISTEN
`

func TestParsePortProbeSS(t *testing.T) {
	ports, err := parsePortProbeOutput(ssFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []ListeningPort{
		{Port: 22, Address: "0.0.0.0", Process: "sshd", PID: 1},
		{Port: 8080, Address: "127.0.0.1", Process: "node", PID: 321},
	}
	if len(ports) != len(want) {
		t.Fatalf("ports = %+v, want %+v", ports, want)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports[%d] = %+v, want %+v", i, ports[i], want[i])
		}
	}
}

func TestParsePortProbeLsof(t *testing.T) {
	ports, err := parsePortProbeOutput(lsofFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []ListeningPort{
		{Port: 22, Address: "*", Process: "sshd", PID: 1},
		{Port: 5432, Address: "127.0.0.1", Process: "python", PID: 99},
		{Port: 8080, Address: "*", Process: "node", PID: 321},
	}
	if len(ports) != len(want) {
		t.Fatalf("ports = %+v, want %+v", ports, want)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports[%d] = %+v, want %+v", i, ports[i], want[i])
		}
	}
}

func TestParsePortProbeNetstatLinux(t *testing.T) {
	ports, err := parsePortProbeOutput(netstatLinuxFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []ListeningPort{
		{Port: 22, Address: "::", Process: "sshd", PID: 1},
		{Port: 631, Address: "127.0.0.1", Process: "cupsd", PID: 123},
	}
	if len(ports) != len(want) {
		t.Fatalf("ports = %+v, want %+v", ports, want)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports[%d] = %+v, want %+v", i, ports[i], want[i])
		}
	}
}

func TestParsePortProbeNetstatDarwin(t *testing.T) {
	ports, err := parsePortProbeOutput(netstatDarwinFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []ListeningPort{
		{Port: 631, Address: "127.0.0.1"},
		{Port: 8080, Address: "::1"},
	}
	if len(ports) != len(want) {
		t.Fatalf("ports = %+v, want %+v", ports, want)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports[%d] = %+v, want %+v", i, ports[i], want[i])
		}
	}
}

func TestParsePortProbeUnavailable(t *testing.T) {
	_, err := parsePortProbeOutput("@@none\n")
	if !errors.Is(err, errs.ErrPortListUnavailable) {
		t.Fatalf("error = %v, want ErrPortListUnavailable", err)
	}
}

func TestParsePortProbeUnknownOutput(t *testing.T) {
	_, err := parsePortProbeOutput("nothing useful\n")
	if !errors.Is(err, errs.ErrPortListUnavailable) {
		t.Fatalf("error = %v, want ErrPortListUnavailable", err)
	}
}

func TestParsePortProbeEmptyList(t *testing.T) {
	ports, err := parsePortProbeOutput("@@ss\nLISTEN 0 1 127.0.0.1:22 0.0.0.0:*\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ports) != 1 || ports[0].Port != 22 || ports[0].Process != "" {
		t.Fatalf("ports = %+v, want one port without a process", ports)
	}
}

func newListeningService(t *testing.T, conn *fakeConn, local CommandRunner) (*ListeningService, *memory.SessionRepository, *sshx.Pool) {
	t.Helper()
	sessions := memory.NewSessionRepository()
	if err := sessions.Create(context.Background(), model.Session{ID: "s1"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	pool := sshx.NewPool()
	if conn != nil {
		pool.SetController("s1", conn)
	}
	return NewListeningService(ListeningServiceConfig{
		Sessions: sessions,
		Pool:     pool,
		Local:    local,
		Probe:    NewPosixPortProbe(),
	}), sessions, pool
}

func TestListeningServiceLocalUsesLocalRunner(t *testing.T) {
	runner := &stubRunner{output: ssFixture}
	svc, _, _ := newListeningService(t, nil, runner)

	ports, err := svc.List(context.Background(), "s1", PortListLocal)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("ports = %+v, want two", ports)
	}
	if runner.calls != 1 {
		t.Fatalf("local runner calls = %d, want 1", runner.calls)
	}
	if !strings.Contains(runner.command, "command -v ss") {
		t.Fatalf("local runner command = %q, want the probe script", runner.command)
	}
	if !strings.Contains(runner.command, "-FpcPn") {
		t.Fatalf("local runner command = %q, want lsof's name field", runner.command)
	}
}

func TestListeningServiceRemoteUsesController(t *testing.T) {
	conn := &fakeConn{runOutput: ssFixture}
	runner := &stubRunner{output: ssFixture}
	svc, _, _ := newListeningService(t, conn, runner)

	ports, err := svc.List(context.Background(), "s1", PortListRemote)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("ports = %+v, want two", ports)
	}
	if conn.runCalls != 1 {
		t.Fatalf("controller run calls = %d, want 1", conn.runCalls)
	}
	if runner.calls != 0 {
		t.Fatalf("local runner calls = %d, want 0", runner.calls)
	}
}

func TestListeningServiceMissingSession(t *testing.T) {
	svc, _, _ := newListeningService(t, nil, &stubRunner{output: ssFixture})
	_, err := svc.List(context.Background(), "missing", PortListLocal)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestListeningServiceMissingController(t *testing.T) {
	svc, _, _ := newListeningService(t, nil, &stubRunner{output: ssFixture})
	_, err := svc.List(context.Background(), "s1", PortListRemote)
	if !errors.Is(err, errs.ErrNoSession) {
		t.Fatalf("error = %v, want ErrNoSession", err)
	}
}

func TestListeningServiceInvalidHost(t *testing.T) {
	svc, _, _ := newListeningService(t, nil, &stubRunner{output: ssFixture})
	_, err := svc.List(context.Background(), "s1", PortListHost("elsewhere"))
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestListeningServiceProbeError(t *testing.T) {
	svc, _, _ := newListeningService(t, nil, &stubRunner{err: errors.New("boom")})
	_, err := svc.List(context.Background(), "s1", PortListLocal)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want the runner error", err)
	}
}
