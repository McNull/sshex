package model

import "time"

type EndpointKind string

const (
	EndpointUnix EndpointKind = "unix"
	EndpointTCP  EndpointKind = "tcp"
)

type Endpoint struct {
	Kind    EndpointKind `json:"kind"`
	Address string       `json:"address"`
}

type SessionState string

const (
	SessionPending SessionState = "pending"
	SessionActive  SessionState = "active"
	SessionClosing SessionState = "closing"
	SessionClosed  SessionState = "closed"
	SessionFailed  SessionState = "failed"
)

type Session struct {
	ID        string
	Origin    string
	User      string
	Host      string
	Port      int
	JumpHosts string
	State     SessionState
	Endpoint  Endpoint
	Token     string
	// RemoteToken authenticates the restricted client installed on the remote
	// host. It grants tunnels and exec but not command management, so commands
	// can only be edited from the origin.
	RemoteToken  string
	Capabilities Capabilities
	ControlPath  string
	RemoteSocket string
	// Shell is the login shell detected on the remote, used to publish shell
	// aliases. It is empty when the shell could not be determined.
	Shell     string
	CreatedAt time.Time
	ClosedAt  time.Time
}
