package cmd

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mcnull/sshex/internal/daemon"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/ui"
	"github.com/spf13/cobra"
)

func TestConnectJumpFlag(t *testing.T) {
	flag := connectCmd.Flags().Lookup("jump")
	if flag == nil {
		t.Fatal("connect --jump flag is not registered")
	}
	if flag.Shorthand != "J" {
		t.Fatalf("connect --jump shorthand = %q, want J", flag.Shorthand)
	}
	if flag.DefValue != "" {
		t.Fatalf("connect --jump default = %q, want empty", flag.DefValue)
	}
}

type connectTestConnection struct{ closed bool }

func (*connectTestConnection) ControlPath() string { return "" }
func (*connectTestConnection) Target() sshx.Target { return sshx.Target{} }
func (*connectTestConnection) Run(context.Context, string, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (*connectTestConnection) AddLocalForward(context.Context, sshx.ForwardSpec) error    { return nil }
func (*connectTestConnection) RemoveLocalForward(context.Context, sshx.ForwardSpec) error { return nil }
func (*connectTestConnection) AddRemoteForward(context.Context, sshx.ForwardSpec) error   { return nil }
func (*connectTestConnection) RemoveRemoteForward(context.Context, sshx.ForwardSpec) error {
	return nil
}
func (*connectTestConnection) AddRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (*connectTestConnection) RemoveRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (*connectTestConnection) Shell(context.Context, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (c *connectTestConnection) Close(context.Context) error {
	c.closed = true
	return nil
}

func TestEstablishMasterAndEnsureBrokerStartsBrokerOnlyAfterSSHConnect(t *testing.T) {
	oldEstablish, oldEnsure := establishConnectMaster, ensureConnectBroker
	t.Cleanup(func() { establishConnectMaster, ensureConnectBroker = oldEstablish, oldEnsure })

	sshErr := errors.New("host unreachable")
	brokerCalled := false
	establishConnectMaster = func(context.Context, sshx.Target, sshx.ConnectOptions) (sshx.Connection, error) {
		return nil, sshErr
	}
	ensureConnectBroker = func(context.Context, string, io.Writer) (daemon.RuntimeInfo, error) {
		brokerCalled = true
		return daemon.RuntimeInfo{}, nil
	}

	if _, _, err := establishMasterAndEnsureBroker(context.Background(), sshx.Target{Host: "offline"}, sshx.ConnectOptions{}, "", io.Discard); !errors.Is(err, sshErr) {
		t.Fatalf("establishMasterAndEnsureBroker() error = %v, want %v", err, sshErr)
	}
	if brokerCalled {
		t.Fatal("broker was started even though SSH connection failed")
	}
}

func TestEstablishMasterAndEnsureBrokerClosesMasterOnBrokerFailure(t *testing.T) {
	oldEstablish, oldEnsure := establishConnectMaster, ensureConnectBroker
	t.Cleanup(func() { establishConnectMaster, ensureConnectBroker = oldEstablish, oldEnsure })

	master := &connectTestConnection{}
	brokerErr := errors.New("broker failed")
	establishConnectMaster = func(context.Context, sshx.Target, sshx.ConnectOptions) (sshx.Connection, error) {
		return master, nil
	}
	ensureConnectBroker = func(context.Context, string, io.Writer) (daemon.RuntimeInfo, error) {
		return daemon.RuntimeInfo{}, brokerErr
	}

	_, _, err := establishMasterAndEnsureBroker(context.Background(), sshx.Target{Host: "reachable"}, sshx.ConnectOptions{}, "", io.Discard)
	if !errors.Is(err, brokerErr) {
		t.Fatalf("establishMasterAndEnsureBroker() error = %v, want %v", err, brokerErr)
	}
	if !master.closed {
		t.Fatal("SSH master was not closed after broker startup failed")
	}
}

func TestCompleteConnectTarget(t *testing.T) {
	orig := resolveConnectHosts
	t.Cleanup(func() { resolveConnectHosts = orig })
	resolveConnectHosts = func() ([]ui.HostCompletion, error) {
		return []ui.HostCompletion{
			{Name: "db", Description: "db.example.com"},
			{Name: "web"},
			{Name: "web2", Description: "web2.example.com"},
		}, nil
	}

	cases := []struct {
		toComplete string
		want       []string
	}{
		{"", []string{"db\tdb.example.com", "web", "web2\tweb2.example.com"}},
		{"web", []string{"web", "web2\tweb2.example.com"}},
		{"nope", nil},
		{"user@web", []string{"user@web", "user@web2\tweb2.example.com"}},
		{"-p", nil},
	}

	for _, tc := range cases {
		comps, directive := completeConnectTarget(connectCmd, nil, tc.toComplete)
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeConnectTarget(%q) directive = %v, want NoFileComp", tc.toComplete, directive)
		}
		if len(comps) != len(tc.want) {
			t.Fatalf("completeConnectTarget(%q) = %v, want %v", tc.toComplete, comps, tc.want)
		}
		for i := range tc.want {
			if comps[i] != tc.want[i] {
				t.Fatalf("completeConnectTarget(%q) = %v, want %v", tc.toComplete, comps, tc.want)
			}
		}
	}
}
