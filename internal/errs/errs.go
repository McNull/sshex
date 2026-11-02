package errs

import "errors"

var (
	ErrNotImplemented = errors.New("not implemented yet")
	ErrInvalidInput   = errors.New("invalid input")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrNoSession      = errors.New("no active session")
	ErrActiveSessions = errors.New("active sshex sessions")
	ErrDisabled       = errors.New("command is disabled")
	// ErrCommandReadOnly is returned when a command mutation is attempted from
	// a remote sshex session, where command definitions are managed by the
	// origin.
	ErrCommandReadOnly = errors.New("commands cannot be edited from a remote sshex session")
	// ErrPortListUnavailable is returned when the host has none of the tools
	// used to list listening TCP ports.
	ErrPortListUnavailable = errors.New("port listing is not available on this host")
)
