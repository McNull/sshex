package cmd

import (
	"testing"

	"github.com/mcnull/sshex/internal/sshhosts"
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

func TestCompleteConnectTarget(t *testing.T) {
	orig := resolveConnectHosts
	t.Cleanup(func() { resolveConnectHosts = orig })
	resolveConnectHosts = func() ([]sshhosts.Host, error) {
		return []sshhosts.Host{
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
