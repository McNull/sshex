package api

import (
	"time"

	"github.com/mcnull/sshex/internal/model"
)

type CreateSessionRequest struct {
	User            string   `json:"user"`
	Host            string   `json:"host"`
	Port            int      `json:"port,omitempty"`
	IdentityFile    string   `json:"identity_file,omitempty"`
	JumpHosts       string   `json:"jump_hosts,omitempty"`
	Options         []string `json:"options,omitempty"`
	SkipCompletions bool     `json:"skip_completions,omitempty"`
	// ControllerControlPath and ConnectionControlPath carry SSH control masters
	// that an interactive client established in the foreground so the broker can
	// adopt them instead of creating its own (which cannot prompt).
	ControllerControlPath string `json:"controller_control_path,omitempty"`
	ConnectionControlPath string `json:"connection_control_path,omitempty"`
}

type SessionResponse struct {
	ID           string             `json:"id"`
	Origin       string             `json:"origin,omitempty"`
	User         string             `json:"user"`
	Host         string             `json:"host"`
	Port         int                `json:"port"`
	JumpHosts    string             `json:"jump_hosts,omitempty"`
	State        model.SessionState `json:"state"`
	Endpoint     model.Endpoint     `json:"endpoint"`
	Capabilities model.Capabilities `json:"capabilities"`
	ControlPath  string             `json:"control_path,omitempty"`
	Connections  int                `json:"connections"`
	Stale        bool               `json:"stale,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
}

type ConnectionResponse struct {
	ID          string                `json:"id"`
	SessionID   string                `json:"session_id"`
	ControlPath string                `json:"control_path,omitempty"`
	State       model.ConnectionState `json:"state"`
	Stale       bool                  `json:"stale,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
}

type CreateSessionResponse struct {
	Session             SessionResponse    `json:"session"`
	Connection          ConnectionResponse `json:"connection"`
	HeartbeatIntervalMS int                `json:"heartbeat_interval_ms"`
}

type CreateTunnelRequest struct {
	LocalPort  int                    `json:"local_port"`
	TargetHost string                 `json:"target_host,omitempty"`
	TargetPort int                    `json:"target_port,omitempty"`
	Direction  model.ForwardDirection `json:"direction,omitempty"`
}

type TunnelResponse struct {
	ID         string                 `json:"id"`
	SessionID  string                 `json:"session_id"`
	LocalPort  int                    `json:"local_port"`
	TargetHost string                 `json:"target_host"`
	TargetPort int                    `json:"target_port"`
	Direction  model.ForwardDirection `json:"direction"`
	State      model.ForwardState     `json:"state"`
	Error      string                 `json:"error,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewSessionResponse(session model.Session) SessionResponse {
	return SessionResponse{
		ID:           session.ID,
		Origin:       session.Origin,
		User:         session.User,
		Host:         session.Host,
		Port:         session.Port,
		JumpHosts:    session.JumpHosts,
		State:        session.State,
		Endpoint:     session.Endpoint,
		Capabilities: session.Capabilities,
		ControlPath:  session.ControlPath,
		CreatedAt:    session.CreatedAt,
	}
}

func NewConnectionResponse(connection model.Connection) ConnectionResponse {
	return ConnectionResponse{
		ID:          connection.ID,
		SessionID:   connection.SessionID,
		ControlPath: connection.ControlPath,
		State:       connection.State,
		CreatedAt:   connection.CreatedAt,
	}
}

func NewCreateSessionResponse(session model.Session, connection model.Connection, heartbeatInterval time.Duration) CreateSessionResponse {
	return CreateSessionResponse{
		Session:             NewSessionResponse(session),
		Connection:          NewConnectionResponse(connection),
		HeartbeatIntervalMS: int(heartbeatInterval / time.Millisecond),
	}
}

func NewTunnelResponse(tunnel model.Tunnel) TunnelResponse {
	return TunnelResponse{
		ID:         tunnel.ID,
		SessionID:  tunnel.SessionID,
		LocalPort:  tunnel.LocalPort,
		TargetHost: tunnel.TargetHost,
		TargetPort: tunnel.TargetPort,
		Direction:  tunnel.Direction,
		State:      tunnel.State,
		Error:      tunnel.Error,
		CreatedAt:  tunnel.CreatedAt,
	}
}

// ExecRequest is the body of an exec request from a remote shell.
type ExecRequest struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
	Cwd  string   `json:"cwd,omitempty"`
	User string   `json:"user,omitempty"`
}

// ExecFrame is one NDJSON frame of an exec response. Command carries the
// rendered command line (sent once, first). Stream is "stdout" or "stderr" for
// data frames; the final frame carries ExitCode.
type ExecFrame struct {
	Command  string `json:"command,omitempty"`
	Stream   string `json:"stream,omitempty"`
	Data     string `json:"data,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Error    string `json:"error,omitempty"`
}

type CreateCommandRequest struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Alias    string `json:"alias,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type UpdateCommandRequest struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Alias    string `json:"alias,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type CommandResponse struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Alias    string `json:"alias,omitempty"`
	Disabled bool   `json:"disabled"`
}

func NewCommandResponse(command model.Command) CommandResponse {
	return CommandResponse{Name: command.Name, Command: command.Command, Alias: command.Alias, Disabled: command.Disabled}
}
