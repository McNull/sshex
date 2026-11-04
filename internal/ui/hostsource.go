package ui

import (
	"github.com/mcnull/sshex/internal/hostfilemgr"
	"github.com/mcnull/sshex/internal/sshconfigmgr"
	"github.com/mcnull/sshex/internal/sshknownhostsmgr"
	"github.com/mcnull/sshex/internal/ui/hosts"
)

type sshHostSource struct{ manager *hosts.HostMgr }

func newSSHHostSource() *sshHostSource {
	return &sshHostSource{manager: &hosts.HostMgr{
		HostFile:   hostfilemgr.NewHostFileMgr(),
		KnownHosts: sshknownhostsmgr.NewSshKnownHostsMgr(),
		SSHConfig:  sshconfigmgr.NewSshConfigMgr(),
	}}
}

func (s *sshHostSource) List() ([]hosts.Candidate, error) { return s.manager.List() }

// HostCompletion is the hostname information needed for CLI completion.
type HostCompletion struct {
	Name        string
	Description string
}

// ConnectHostCompletions returns UI host candidates for CLI completion without
// exposing the UI's source managers to the command package.
func ConnectHostCompletions() ([]HostCompletion, error) {
	hosts, err := newSSHHostSource().List()
	if err != nil {
		return nil, err
	}
	out := make([]HostCompletion, len(hosts))
	for i, host := range hosts {
		out[i] = HostCompletion{Name: host.Name, Description: host.Description}
	}
	return out, nil
}
