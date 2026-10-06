package model

import "time"

type ConnectionState string

const (
	ConnectionActive ConnectionState = "active"
	ConnectionClosed ConnectionState = "closed"
)

// Connection is a single SSH connection from the origin to the remote. One or
// more connections make up a Session; the interactive shell of an
// `sshex connect` runs over one connection.
type Connection struct {
	ID          string
	SessionID   string
	ControlPath string
	State       ConnectionState
	CreatedAt   time.Time
	ClosedAt    time.Time
}
