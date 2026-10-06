package cmd

import (
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/spf13/cobra"
)

func newDirectionCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "add"}
	cmd.Flags().BoolP("local", "L", false, "")
	cmd.Flags().BoolP("remote", "R", false, "")
	_ = cmd.ParseFlags(args)
	return cmd
}

func TestTunnelDirection(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fallback model.ForwardDirection
		want     model.ForwardDirection
		wantErr  bool
	}{
		{name: "default", fallback: model.ForwardLocal, want: model.ForwardLocal},
		{name: "local flag", args: []string{"-L"}, fallback: model.ForwardLocal, want: model.ForwardLocal},
		{name: "remote flag", args: []string{"-R"}, fallback: model.ForwardLocal, want: model.ForwardRemote},
		{name: "rm default", fallback: "", want: ""},
		{name: "both flags", args: []string{"-L", "-R"}, fallback: model.ForwardLocal, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tunnelDirection(newDirectionCmd(tt.args...), tt.fallback)
			if tt.wantErr {
				if err == nil {
					t.Fatal("tunnelDirection() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("tunnelDirection() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("tunnelDirection() = %q, want %q", got, tt.want)
			}
		})
	}
}
