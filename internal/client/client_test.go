package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/model"
)

func newUnixServer(t *testing.T, handler http.Handler) (model.Endpoint, func()) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return model.Endpoint{Kind: model.EndpointUnix, Address: sock}, func() { _ = srv.Close() }
}

func TestClientListSessions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]api.SessionResponse{{ID: "s1", Host: "ssh01", Origin: "laptop"}})
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "tok")
	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" || sessions[0].Origin != "laptop" {
		t.Fatalf("ListSessions() = %+v", sessions)
	}
}

func TestClientHeartbeat(t *testing.T) {
	var gotPath, gotMethod string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions/{id}/connections/{connID}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "tok")
	if err := c.Heartbeat(context.Background(), "s1", "c1"); err != nil {
		t.Fatalf("Heartbeat() error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("Heartbeat() method = %q, want POST", gotMethod)
	}
	if gotPath != "/sessions/s1/connections/c1/heartbeat" {
		t.Fatalf("Heartbeat() path = %q", gotPath)
	}
}

func TestClientForSessionAndClose(t *testing.T) {
	var gotPath, gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]api.TunnelResponse{{LocalPort: 2222}})
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "secret")
	clone := c.ForSession("other")
	if clone.SessionID() != "other" {
		t.Fatalf("ForSession() id = %q, want other", clone.SessionID())
	}
	if c.SessionID() != "" {
		t.Fatalf("ForSession() mutated the original client: id = %q", c.SessionID())
	}

	dropped, err := clone.CloseSession(context.Background(), "other")
	if err != nil {
		t.Fatalf("CloseSession() error: %v", err)
	}
	if gotPath != "/sessions/other" {
		t.Fatalf("CloseSession() path = %q, want /sessions/other", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("CloseSession() auth = %q", gotAuth)
	}
	if len(dropped) != 1 || dropped[0].LocalPort != 2222 {
		t.Fatalf("CloseSession() dropped = %+v", dropped)
	}
}

func TestClientExec(t *testing.T) {
	var gotPath string
	var gotRequest api.ExecRequest
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions/{id}/exec", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotRequest)
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(api.ExecFrame{Command: `code --folder-uri "vscode-remote://ssh-remote+ssh01/remote"`})
		_ = encoder.Encode(api.ExecFrame{Stream: "stdout", Data: "hello "})
		_ = encoder.Encode(api.ExecFrame{Stream: "stderr", Data: "warn"})
		code := 4
		_ = encoder.Encode(api.ExecFrame{ExitCode: &code})
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "tok").ForSession("s1")
	var notice, stdout, stderr bytes.Buffer
	code, err := c.Exec(context.Background(), api.ExecRequest{Name: "code", Args: []string{"/p"}, Cwd: "/remote"}, &notice, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if code != 4 {
		t.Fatalf("Exec() code = %d, want 4", code)
	}
	if gotPath != "/sessions/s1/exec" {
		t.Fatalf("Exec() path = %q", gotPath)
	}
	if gotRequest.Name != "code" || gotRequest.Cwd != "/remote" || len(gotRequest.Args) != 1 {
		t.Fatalf("Exec() request = %+v", gotRequest)
	}
	if notice.String() != "Executing `code --folder-uri \"vscode-remote://ssh-remote+ssh01/remote\"` ...\n" {
		t.Fatalf("Exec() notice = %q", notice.String())
	}
	if stdout.String() != "hello " || stderr.String() != "warn" {
		t.Fatalf("Exec() output = %q / %q", stdout.String(), stderr.String())
	}
}

func TestClientExecError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions/{id}/exec", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadRequest)
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "tok").ForSession("s1")
	if _, err := c.Exec(context.Background(), api.ExecRequest{Name: "code"}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("Exec() expected error, got nil")
	}
}

func TestClientCommands(t *testing.T) {
	var paths []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /commands", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		_ = json.NewEncoder(w).Encode([]api.CommandResponse{{Name: "code", Command: "code"}})
	})
	mux.HandleFunc("POST /commands", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		_ = json.NewEncoder(w).Encode(api.CommandResponse{Name: "code", Command: "code"})
	})
	mux.HandleFunc("PUT /commands/{name}", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		_ = json.NewEncoder(w).Encode(api.CommandResponse{Name: "code", Command: "code", Alias: "ed"})
	})
	mux.HandleFunc("DELETE /commands/{name}", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /commands/{name}/{action}", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	endpoint, closeFn := newUnixServer(t, mux)
	defer closeFn()

	c := NewForEndpoint(endpoint, "tok")
	if commands, err := c.ListCommands(context.Background()); err != nil || len(commands) != 1 {
		t.Fatalf("ListCommands() = %+v, %v", commands, err)
	}
	if _, err := c.AddCommand(context.Background(), "code", "code", "ed", false); err != nil {
		t.Fatalf("AddCommand() error: %v", err)
	}
	if _, err := c.UpdateCommand(context.Background(), "code", api.UpdateCommandRequest{Name: "code", Command: "code", Alias: "ed"}); err != nil {
		t.Fatalf("UpdateCommand() error: %v", err)
	}
	if err := c.SetCommandDisabled(context.Background(), "code", true); err != nil {
		t.Fatalf("SetCommandDisabled() error: %v", err)
	}
	if err := c.RemoveCommand(context.Background(), "code"); err != nil {
		t.Fatalf("RemoveCommand() error: %v", err)
	}

	want := []string{"GET /commands", "POST /commands", "PUT /commands/code", "POST /commands/code/disable", "DELETE /commands/code"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %v, want %v", paths, want)
		}
	}
}
