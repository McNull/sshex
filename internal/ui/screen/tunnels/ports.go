package tunnels

// PortHost selects the machine whose listening ports the add form shows.
type PortHost string

const (
	PortHostLocal  PortHost = "local"
	PortHostRemote PortHost = "remote"
)

// Port is one listening TCP port shown below the spec field.
type Port struct {
	Port    int
	Address string
	Process string
	PID     int
}

// PortSource lists listening ports for one side of a session.
type PortSource interface {
	List(sessionID string, host PortHost) ([]Port, error)
}

// portHostForDirection returns the side that owns the service being exposed:
// a local forward targets the remote host and a remote forward targets the
// origin.
func portHostForDirection(direction Direction) PortHost {
	if direction == DirectionRemote {
		return PortHostLocal
	}
	return PortHostRemote
}
