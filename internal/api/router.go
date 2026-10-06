package api

import "net/http"

func NewRouter(handlers *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)

	mux.HandleFunc("POST /sessions", handlers.CreateSession)
	mux.HandleFunc("GET /sessions", handlers.ListSessions)
	mux.HandleFunc("GET /sessions/{id}", handlers.GetSession)
	mux.HandleFunc("DELETE /sessions/{id}", handlers.DeleteSession)
	mux.HandleFunc("GET /sessions/{id}/connections", handlers.ListConnections)
	mux.HandleFunc("POST /sessions/{id}/connections/{connID}/detach", handlers.DetachConnection)
	mux.HandleFunc("POST /sessions/{id}/connections/{connID}/heartbeat", handlers.HeartbeatConnection)

	mux.HandleFunc("GET /sessions/{id}/tunnels", handlers.ListTunnels)
	mux.HandleFunc("POST /sessions/{id}/tunnels", handlers.CreateTunnel)
	mux.HandleFunc("DELETE /sessions/{id}/tunnels/{port}", handlers.DeleteTunnel)

	mux.HandleFunc("POST /sessions/{id}/exec", handlers.Exec)

	mux.HandleFunc("GET /commands", handlers.ListCommands)
	mux.HandleFunc("POST /commands", handlers.CreateCommand)
	mux.HandleFunc("PUT /commands/{name}", handlers.UpdateCommand)
	mux.HandleFunc("DELETE /commands/{name}", handlers.DeleteCommand)
	mux.HandleFunc("POST /commands/{name}/enable", handlers.EnableCommand)
	mux.HandleFunc("POST /commands/{name}/disable", handlers.DisableCommand)

	return mux
}
