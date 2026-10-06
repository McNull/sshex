package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/control/unix"
	"github.com/mcnull/sshex/internal/local"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/repository/memory"
	"github.com/mcnull/sshex/internal/service"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
)

const (
	testToken       = "test-token"
	restrictedToken = "restricted-token"
)

func newTestRouter() http.Handler {
	return newTestEnv().handler
}

type testEnv struct {
	handler     http.Handler
	sessions    *memory.SessionRepository
	connections *memory.ConnectionRepository
	commands    *memory.CommandRepository
}

func newTestEnv() testEnv {
	sessionRepo := memory.NewSessionRepository()
	connectionRepo := memory.NewConnectionRepository()
	tunnelRepo := memory.NewTunnelRepository()
	commandRepo := memory.NewCommandRepository()
	transport := openssh.NewTransport()
	installer := remoteinstall.NewLinuxInstaller()
	auth := control.NewAuthenticator()
	auth.Register(testToken, model.Capabilities{model.CapabilityTunnels, model.CapabilityExec, model.CapabilityCommands}, "")
	auth.Register(restrictedToken, model.Capabilities{model.CapabilityTunnels, model.CapabilityExec}, "")
	provider := unix.NewProvider("")
	pool := sshx.NewPool()
	ids := func() string { return "test-id" }

	sessions := service.NewSessionService(service.SessionServiceConfig{
		Sessions:    sessionRepo,
		Connections: connectionRepo,
		Tunnels:     tunnelRepo,
		Transport:   transport,
		Control:     provider,
		Installer:   installer,
		Auth:        auth,
		IDs:         ids,
		Pool:        pool,
	})
	services := Services{
		Sessions: sessions,
		Tunnels: service.NewTunnelService(service.TunnelServiceConfig{
			Sessions:  sessionRepo,
			Tunnels:   tunnelRepo,
			Transport: transport,
			IDs:       ids,
			Pool:      pool,
		}),
		Commands: service.NewCommandService(service.CommandServiceConfig{
			Commands: commandRepo,
			Executor: local.NewExecutor(),
		}),
	}
	return testEnv{
		handler:     NewRouter(NewHandlers(services, auth)),
		sessions:    sessionRepo,
		connections: connectionRepo,
		commands:    commandRepo,
	}
}

func do(t *testing.T, handler http.Handler, method, path, body string, authorized bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorized {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/health", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body = %q, want status ok", rec.Body.String())
	}
}

func TestListSessionsEmpty(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/sessions", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", rec.Body.String())
	}
}

func TestGetSessionNotFound(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/sessions/missing", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestCreateSessionRequiresHost(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodPost, "/sessions", `{}`, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSessionInvalidBody(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodPost, "/sessions", `{`, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestTunnelEndpointsRequireAuth(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/sessions/s1/tunnels", "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestListTunnelsEmpty(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/sessions/s1/tunnels", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", rec.Body.String())
	}
}

func TestCreateTunnelMissingSession(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodPost, "/sessions/s1/tunnels", `{"local_port":8080}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteTunnelInvalidPort(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodDelete, "/sessions/s1/tunnels/abc", "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDeleteSessionMissing(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodDelete, "/sessions/s1", "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodDelete, "/health", "", false)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHeartbeatConnection(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv()
	if err := env.sessions.Create(ctx, model.Session{ID: "s1", State: model.SessionActive}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := env.connections.Create(ctx, model.Connection{ID: "c1", SessionID: "s1"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := do(t, env.handler, http.MethodPost, "/sessions/s1/connections/c1/heartbeat", "", true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, env.handler, http.MethodPost, "/sessions/s1/connections/missing/heartbeat", "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown heartbeat status = %d, want 404", rec.Code)
	}

	rec = do(t, env.handler, http.MethodPost, "/sessions/s1/connections/c1/heartbeat", "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated heartbeat status = %d, want 401", rec.Code)
	}
}

func TestListConnections(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv()
	if err := env.connections.Create(ctx, model.Connection{ID: "c1", SessionID: "s1"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := do(t, env.handler, http.MethodGet, "/sessions/s1/connections", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"id":"c1"`) {
		t.Fatalf("body = %q, want connection c1", rec.Body.String())
	}
}

func seedSession(t *testing.T, env testEnv) {
	t.Helper()
	if err := env.sessions.Create(context.Background(), model.Session{ID: "s1", State: model.SessionActive, Host: "ssh01", User: "bob", Port: 22, Origin: "laptop"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestExecStreamsOutput(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv()
	seedSession(t, env)
	if err := env.commands.Create(ctx, model.Command{Name: "code", Command: `printf '%s' "${@:-${REMOTE_CWD}}"`}); err != nil {
		t.Fatalf("seed command: %v", err)
	}

	rec := do(t, env.handler, http.MethodPost, "/sessions/s1/exec", `{"name":"code","cwd":"/remote"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	frames := decodeFrames(t, rec.Body.String())
	if len(frames) < 3 {
		t.Fatalf("frames = %+v, want command, stdout and exit", frames)
	}
	if frames[0].Command != `printf '%s' "/remote"` {
		t.Fatalf("first frame command = %q, want rendered command", frames[0].Command)
	}
	if frames[1].Stream != "stdout" || frames[1].Data != "/remote" {
		t.Fatalf("stdout frame = %+v", frames[1])
	}
	last := frames[len(frames)-1]
	if last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("exit frame = %+v, want exit_code 0", last)
	}
}

func decodeFrames(t *testing.T, body string) []ExecFrame {
	t.Helper()
	var frames []ExecFrame
	decoder := json.NewDecoder(strings.NewReader(body))
	for {
		var frame ExecFrame
		if err := decoder.Decode(&frame); err != nil {
			break
		}
		frames = append(frames, frame)
	}
	return frames
}

func TestExecMissingCommand(t *testing.T) {
	env := newTestEnv()
	seedSession(t, env)
	rec := do(t, env.handler, http.MethodPost, "/sessions/s1/exec", `{"name":"nope"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestExecDisabledCommand(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv()
	seedSession(t, env)
	if err := env.commands.Create(ctx, model.Command{Name: "code", Command: "code", Disabled: true}); err != nil {
		t.Fatalf("seed command: %v", err)
	}
	rec := do(t, env.handler, http.MethodPost, "/sessions/s1/exec", `{"name":"code"}`, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestExecRequiresAuth(t *testing.T) {
	env := newTestEnv()
	seedSession(t, env)
	rec := do(t, env.handler, http.MethodPost, "/sessions/s1/exec", `{"name":"code"}`, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCommandEndpoints(t *testing.T) {
	env := newTestEnv()

	rec := do(t, env.handler, http.MethodPost, "/commands", `{"name":"code","command":"code --folder-uri","alias":"ed"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"code"`) || !strings.Contains(rec.Body.String(), `"alias":"ed"`) {
		t.Fatalf("create body = %q", rec.Body.String())
	}

	rec = do(t, env.handler, http.MethodGet, "/commands", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"command":"code --folder-uri"`) {
		t.Fatalf("list body = %q", rec.Body.String())
	}

	rec = do(t, env.handler, http.MethodPut, "/commands/code", `{"name":"editor","command":"editor","alias":"ed"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"editor"`) {
		t.Fatalf("update body = %q", rec.Body.String())
	}
	stored, _ := env.commands.Get(context.Background(), "editor")
	if stored.Command != "editor" || stored.Alias != "ed" {
		t.Fatalf("updated command = %+v", stored)
	}

	if rec = do(t, env.handler, http.MethodPost, "/commands/editor/disable", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	command, _ := env.commands.Get(context.Background(), "editor")
	if !command.Disabled {
		t.Fatal("disable did not persist")
	}
	if rec = do(t, env.handler, http.MethodPost, "/commands/editor/enable", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("enable status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	if rec = do(t, env.handler, http.MethodDelete, "/commands/editor", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	if rec = do(t, env.handler, http.MethodDelete, "/commands/editor", "", true); rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404", rec.Code)
	}
}

func TestCommandEndpointsRequireAuth(t *testing.T) {
	env := newTestEnv()
	rec := do(t, env.handler, http.MethodGet, "/commands", "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func doWithToken(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCommandMutationsRequireCommandsCapability(t *testing.T) {
	env := newTestEnv()

	if rec := doWithToken(t, env.handler, http.MethodGet, "/commands", "", restrictedToken); rec.Code != http.StatusOK {
		t.Fatalf("list with restricted token status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/commands", `{"name":"code","command":"code"}`},
		{http.MethodPut, "/commands/code", `{"name":"code","command":"code"}`},
		{http.MethodDelete, "/commands/code", ""},
		{http.MethodPost, "/commands/code/disable", ""},
		{http.MethodPost, "/commands/code/enable", ""},
	}
	for _, tc := range cases {
		rec := doWithToken(t, env.handler, tc.method, tc.path, tc.body, restrictedToken)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with restricted token status = %d, want 401: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
