package model

type Capability string

const (
	CapabilityTunnels  Capability = "tunnels"
	CapabilityExec     Capability = "exec"
	CapabilityCommands Capability = "commands"
)

func (c Capability) Valid() bool {
	switch c {
	case CapabilityTunnels, CapabilityExec, CapabilityCommands:
		return true
	default:
		return false
	}
}

// Capabilities is the set of capabilities granted to a token.
type Capabilities []Capability

func (cs Capabilities) Contains(c Capability) bool {
	for _, candidate := range cs {
		if candidate == c {
			return true
		}
	}
	return false
}
