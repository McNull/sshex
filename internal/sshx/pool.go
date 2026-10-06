package sshx

import "sync"

// Pool tracks the live SSH connections owned by the service. Each session has
// one controller connection that owns the control channel and every tunnel of
// the session, plus zero or more interactive connections (one per
// `sshex connect`).
type Pool struct {
	mu          sync.RWMutex
	controllers map[string]Connection
	connections map[string]Connection
}

func NewPool() *Pool {
	return &Pool{
		controllers: make(map[string]Connection),
		connections: make(map[string]Connection),
	}
}

// SetController records the controller connection for a session.
func (p *Pool) SetController(sessionID string, conn Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.controllers[sessionID] = conn
}

// Controller returns the controller connection for a session.
func (p *Pool) Controller(sessionID string) (Connection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.controllers[sessionID]
	return conn, ok
}

// RemoveController drops the controller connection for a session.
func (p *Pool) RemoveController(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.controllers, sessionID)
}

// SetConnection records an interactive connection.
func (p *Pool) SetConnection(connectionID string, conn Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connections[connectionID] = conn
}

// Connection returns an interactive connection by id.
func (p *Pool) Connection(connectionID string) (Connection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.connections[connectionID]
	return conn, ok
}

// RemoveConnection drops an interactive connection.
func (p *Pool) RemoveConnection(connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.connections, connectionID)
}
