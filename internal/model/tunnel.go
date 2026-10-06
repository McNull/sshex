package model

import "time"

type ForwardDirection string

const (
	ForwardLocal  ForwardDirection = "local"
	ForwardRemote ForwardDirection = "remote"
)

type ForwardSource string

const (
	SourceManual   ForwardSource = "manual"
	SourceDetected ForwardSource = "detected"
	SourceConfig   ForwardSource = "config"
)

type ForwardState string

const (
	ForwardPending ForwardState = "pending"
	ForwardActive  ForwardState = "active"
	ForwardFailed  ForwardState = "failed"
	ForwardStopped ForwardState = "stopped"
)

type Tunnel struct {
	ID         string
	SessionID  string
	LocalPort  int
	TargetHost string
	TargetPort int
	Direction  ForwardDirection
	Source     ForwardSource
	State      ForwardState
	Error      string
	CreatedAt  time.Time
}
