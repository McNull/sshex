package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/service"
)

type Services struct {
	Sessions *service.SessionService
	Tunnels  *service.TunnelService
	Commands *service.CommandService
}

type Handlers struct {
	services Services
	auth     *control.Authenticator
}

func NewHandlers(services Services, auth *control.Authenticator) *Handlers {
	return &Handlers{services: services, auth: auth}
}

func (h *Handlers) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if !decode(w, r, &req) {
		return
	}
	session, connection, err := h.services.Sessions.Connect(r.Context(), service.ConnectRequest{
		User:                  req.User,
		Host:                  req.Host,
		Port:                  req.Port,
		IdentityFile:          req.IdentityFile,
		JumpHosts:             req.JumpHosts,
		Options:               req.Options,
		SkipCompletions:       req.SkipCompletions,
		ControllerControlPath: req.ControllerControlPath,
		ConnectionControlPath: req.ConnectionControlPath,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, NewCreateSessionResponse(session, connection, h.services.Sessions.HeartbeatInterval()))
}

func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.services.Sessions.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		resp := h.sessionResponse(r, session)
		responses = append(responses, resp)
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.services.Sessions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.sessionResponse(r, session))
}

func (h *Handlers) sessionResponse(r *http.Request, session model.Session) SessionResponse {
	resp := NewSessionResponse(session)
	resp.Connections = h.services.Sessions.ConnectionCount(r.Context(), session.ID)
	if connections, err := h.services.Sessions.Connections(r.Context(), session.ID); err == nil {
		for _, connection := range connections {
			if h.services.Sessions.IsStale(connection.ID) {
				resp.Stale = true
				break
			}
		}
	}
	return resp
}

func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	connections, err := h.services.Sessions.Connections(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]ConnectionResponse, 0, len(connections))
	for _, connection := range connections {
		resp := NewConnectionResponse(connection)
		resp.Stale = h.services.Sessions.IsStale(connection.ID)
		responses = append(responses, resp)
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *Handlers) HeartbeatConnection(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	if err := h.services.Sessions.Heartbeat(r.Context(), r.PathValue("id"), r.PathValue("connID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	dropped, teardown, err := h.services.Sessions.Close(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]TunnelResponse, 0, len(dropped))
	for _, tunnel := range dropped {
		responses = append(responses, NewTunnelResponse(tunnel))
	}
	writeJSON(w, http.StatusOK, responses)
	if teardown {
		go h.services.Sessions.NotifyTeardown()
	}
}

func (h *Handlers) DetachConnection(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	dropped, teardown, err := h.services.Sessions.Detach(r.Context(), r.PathValue("id"), r.PathValue("connID"))
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]TunnelResponse, 0, len(dropped))
	for _, tunnel := range dropped {
		responses = append(responses, NewTunnelResponse(tunnel))
	}
	writeJSON(w, http.StatusOK, responses)
	if teardown {
		go h.services.Sessions.NotifyTeardown()
	}
}

func (h *Handlers) ListTunnels(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	tunnels, err := h.services.Tunnels.List(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]TunnelResponse, 0, len(tunnels))
	for _, tunnel := range tunnels {
		responses = append(responses, NewTunnelResponse(tunnel))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *Handlers) CreateTunnel(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	var req CreateTunnelRequest
	if !decode(w, r, &req) {
		return
	}
	tunnel, err := h.services.Tunnels.Add(r.Context(), service.AddTunnelRequest{
		SessionID:  r.PathValue("id"),
		LocalPort:  req.LocalPort,
		TargetHost: req.TargetHost,
		TargetPort: req.TargetPort,
		Direction:  req.Direction,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, NewTunnelResponse(tunnel))
}

func (h *Handlers) DeleteTunnel(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityTunnels, r.PathValue("id")) {
		return
	}
	localPort, err := strconv.Atoi(r.PathValue("port"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid port"})
		return
	}
	direction := model.ForwardDirection(r.URL.Query().Get("direction"))
	if direction != "" && direction != model.ForwardLocal && direction != model.ForwardRemote {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid direction"})
		return
	}
	if err := h.services.Tunnels.Remove(r.Context(), r.PathValue("id"), localPort, direction); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Exec(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !h.authorize(w, r, model.CapabilityExec, sessionID) {
		return
	}
	var req ExecRequest
	if !decode(w, r, &req) {
		return
	}
	session, err := h.services.Sessions.Get(r.Context(), sessionID)
	if err != nil {
		writeError(w, err)
		return
	}

	flusher, _ := w.(http.Flusher)
	sink := newExecSink(w, flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	code, err := h.services.Commands.Execute(r.Context(), session, service.ExecRequest{
		SessionID: sessionID,
		Name:      req.Name,
		Args:      req.Args,
		Cwd:       req.Cwd,
		User:      req.User,
	}, service.ExecOutput{
		Command: sink.commandWriter(),
		Stdout:  sink.writer("stdout"),
		Stderr:  sink.writer("stderr"),
	})
	if err != nil {
		if !sink.written() {
			writeError(w, err)
			return
		}
		_ = sink.write(ExecFrame{Stream: "stderr", Error: err.Error()})
		return
	}
	_ = sink.write(ExecFrame{ExitCode: &code})
}

func (h *Handlers) ListCommands(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityExec, "") {
		return
	}
	commands, err := h.services.Commands.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	responses := make([]CommandResponse, 0, len(commands))
	for _, command := range commands {
		responses = append(responses, NewCommandResponse(command))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *Handlers) CreateCommand(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityCommands, "") {
		return
	}
	var req CreateCommandRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.services.Commands.Add(r.Context(), req.Name, req.Command, req.Alias, req.Disabled); err != nil {
		writeError(w, err)
		return
	}
	command, err := h.services.Commands.Get(r.Context(), req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, NewCommandResponse(command))
}

func (h *Handlers) UpdateCommand(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityCommands, "") {
		return
	}
	name := r.PathValue("name")
	var req UpdateCommandRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.services.Commands.Update(r.Context(), name, model.Command{
		Name:     req.Name,
		Command:  req.Command,
		Alias:    req.Alias,
		Disabled: req.Disabled,
	}); err != nil {
		writeError(w, err)
		return
	}
	command, err := h.services.Commands.Get(r.Context(), req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, NewCommandResponse(command))
}

func (h *Handlers) DeleteCommand(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, model.CapabilityCommands, "") {
		return
	}
	if err := h.services.Commands.Remove(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) EnableCommand(w http.ResponseWriter, r *http.Request) {
	h.setCommandDisabled(w, r, false)
}

func (h *Handlers) DisableCommand(w http.ResponseWriter, r *http.Request) {
	h.setCommandDisabled(w, r, true)
}

func (h *Handlers) setCommandDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	if !h.authorize(w, r, model.CapabilityCommands, "") {
		return
	}
	if err := h.services.Commands.SetDisabled(r.Context(), r.PathValue("name"), disabled); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// execSink serializes NDJSON frames to a streaming HTTP response. stdout and
// stderr writers share one mutex so concurrent shell writes do not interleave.
type execSink struct {
	mu      sync.Mutex
	w       io.Writer
	flusher http.Flusher
	hasData bool
}

func newExecSink(w io.Writer, flusher http.Flusher) *execSink {
	return &execSink{w: w, flusher: flusher}
}

func (s *execSink) write(frame ExecFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasData = true
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *execSink) written() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasData
}

func (s *execSink) writer(stream string) io.Writer {
	return &execStreamWriter{sink: s, stream: stream}
}

// commandWriter emits a single frame carrying the rendered command line.
func (s *execSink) commandWriter() io.Writer {
	return &execCommandWriter{sink: s}
}

type execCommandWriter struct {
	sink *execSink
}

func (w *execCommandWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.sink.write(ExecFrame{Command: string(p)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

type execStreamWriter struct {
	sink   *execSink
	stream string
}

func (w *execStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.sink.write(ExecFrame{Stream: w.stream, Data: string(p)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (h *Handlers) authorize(w http.ResponseWriter, r *http.Request, capability model.Capability, sessionID string) bool {
	if err := h.auth.AuthorizeForSession(r, capability, sessionID); err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
		return false
	}
	return true
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errs.ErrNotImplemented):
		status = http.StatusNotImplemented
	case errors.Is(err, errs.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, errs.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, repository.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, repository.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, errs.ErrDisabled):
		status = http.StatusConflict
	}
	writeJSON(w, status, ErrorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
