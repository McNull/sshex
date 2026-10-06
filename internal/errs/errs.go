package errs

import "errors"

var (
	ErrNotImplemented = errors.New("not implemented yet")
	ErrInvalidInput   = errors.New("invalid input")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrNoSession      = errors.New("no active session")
	ErrActiveSessions = errors.New("active sshex sessions")
	ErrDisabled       = errors.New("command is disabled")
)
