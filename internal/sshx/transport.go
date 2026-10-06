package sshx

import (
	"context"
	"fmt"
	"io"
)

type Target struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
	JumpHosts    string
	Options      []string
}

func (t Target) String() string {
	if t.User != "" {
		return t.User + "@" + t.Host
	}
	return t.Host
}

type ForwardSpec struct {
	BindAddress string
	LocalPort   int
	TargetHost  string
	TargetPort  int
}

func (f ForwardSpec) LocalArg() string {
	bind := f.BindAddress
	if bind == "" {
		bind = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d:%s:%d", bind, f.LocalPort, f.TargetHost, f.TargetPort)
}

type RemoteSocketForwardSpec struct {
	RemotePath string
	LocalPath  string
}

func (f RemoteSocketForwardSpec) Arg() string {
	return f.RemotePath + ":" + f.LocalPath
}

type ConnectOptions struct {
	ControlPath string
	SessionID   string
	// Stderr, when non-nil, receives the SSH master's stdio so an interactive
	// client can answer host-key and authentication prompts. It must be an
	// *os.File to avoid a capture pipe that a daemonized ProxyJump helper would
	// hold open; other writers fall back to non-interactive capture.
	Stderr io.Writer
}

type Connection interface {
	ControlPath() string
	Target() Target
	Run(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error
	AddLocalForward(ctx context.Context, spec ForwardSpec) error
	RemoveLocalForward(ctx context.Context, spec ForwardSpec) error
	AddRemoteForward(ctx context.Context, spec ForwardSpec) error
	RemoveRemoteForward(ctx context.Context, spec ForwardSpec) error
	AddRemoteSocketForward(ctx context.Context, spec RemoteSocketForwardSpec) error
	RemoveRemoteSocketForward(ctx context.Context, spec RemoteSocketForwardSpec) error
	Shell(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error
	Close(ctx context.Context) error
}

type Transport interface {
	Connect(ctx context.Context, target Target, opts ConnectOptions) (Connection, error)
	// Adopt reuses an existing SSH control master, verifying that it is alive.
	Adopt(ctx context.Context, target Target, opts ConnectOptions) (Connection, error)
	Attach(target Target, opts ConnectOptions) Connection
}
