package ui

import "github.com/mcnull/sshex/internal/sshhosts"

// sshHostSource adapts sshhosts.Resolve for the Connect screen. The screen
// depends on its own narrow interface, so tests can replace this source.
type sshHostSource struct{}

func (sshHostSource) Resolve() ([]sshhosts.Host, error) {
	return sshhosts.Resolve(sshhosts.Options{})
}
