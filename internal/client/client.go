package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
)

type Client struct {
	http      *http.Client
	base      string
	token     string
	sessionID string
	target    string
}

func New(file sessionfile.File) *Client {
	return newClient(file.Endpoint, file.Token, file.ID, file.Target)
}

// NewForEndpoint builds a client for a bare endpoint and token, used to talk to
// the local broker before any session file exists.
func NewForEndpoint(endpoint model.Endpoint, token string) *Client {
	return newClient(endpoint, token, "", "")
}

func newClient(endpoint model.Endpoint, token, sessionID, target string) *Client {
	transport := &http.Transport{}
	if endpoint.Kind == model.EndpointUnix {
		address := endpoint.Address
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", address)
		}
	}
	return &Client{
		http:      &http.Client{Transport: transport},
		base:      "http://sshex",
		token:     token,
		sessionID: sessionID,
		target:    target,
	}
}

func (c *Client) SessionID() string { return c.sessionID }

func (c *Client) Target() string { return c.target }

// ForSession returns a copy of the client that addresses a different session.
// The endpoint and token are shared, because a broker's token authorizes every
// session of the same origin.
func (c *Client) ForSession(id string) *Client {
	clone := *c
	clone.sessionID = id
	return &clone
}

func (c *Client) CloseSession(ctx context.Context, id string) ([]api.TunnelResponse, error) {
	var tunnels []api.TunnelResponse
	err := c.request(ctx, http.MethodDelete, "/sessions/"+id, nil, &tunnels)
	return tunnels, err
}

func (c *Client) CreateSession(ctx context.Context, req api.CreateSessionRequest) (api.CreateSessionResponse, error) {
	var session api.CreateSessionResponse
	err := c.request(ctx, http.MethodPost, "/sessions", req, &session)
	return session, err
}

func (c *Client) DetachConnection(ctx context.Context, sessionID, connectionID string) ([]api.TunnelResponse, error) {
	var tunnels []api.TunnelResponse
	err := c.request(ctx, http.MethodPost, "/sessions/"+sessionID+"/connections/"+connectionID+"/detach", nil, &tunnels)
	return tunnels, err
}

// Heartbeat keeps a connection alive on the broker.
func (c *Client) Heartbeat(ctx context.Context, sessionID, connectionID string) error {
	return c.request(ctx, http.MethodPost, "/sessions/"+sessionID+"/connections/"+connectionID+"/heartbeat", nil, nil)
}

func (c *Client) ListConnections(ctx context.Context, sessionID string) ([]api.ConnectionResponse, error) {
	var connections []api.ConnectionResponse
	err := c.request(ctx, http.MethodGet, "/sessions/"+sessionID+"/connections", nil, &connections)
	return connections, err
}

func (c *Client) ListSessions(ctx context.Context) ([]api.SessionResponse, error) {
	var sessions []api.SessionResponse
	err := c.request(ctx, http.MethodGet, "/sessions", nil, &sessions)
	return sessions, err
}

func (c *Client) GetSession(ctx context.Context) (api.SessionResponse, error) {
	var session api.SessionResponse
	err := c.request(ctx, http.MethodGet, "/sessions/"+c.sessionID, nil, &session)
	return session, err
}

func (c *Client) ListTunnels(ctx context.Context) ([]api.TunnelResponse, error) {
	var tunnels []api.TunnelResponse
	err := c.request(ctx, http.MethodGet, "/sessions/"+c.sessionID+"/tunnels", nil, &tunnels)
	return tunnels, err
}

func (c *Client) AddTunnel(ctx context.Context, localPort int, targetHost string, targetPort int, direction model.ForwardDirection) (api.TunnelResponse, error) {
	body := api.CreateTunnelRequest{LocalPort: localPort, TargetHost: targetHost, TargetPort: targetPort, Direction: direction}
	var tunnel api.TunnelResponse
	err := c.request(ctx, http.MethodPost, "/sessions/"+c.sessionID+"/tunnels", body, &tunnel)
	return tunnel, err
}

func (c *Client) RemoveTunnel(ctx context.Context, localPort int, direction model.ForwardDirection) error {
	path := "/sessions/" + c.sessionID + "/tunnels/" + strconv.Itoa(localPort)
	if direction != "" {
		path += "?direction=" + string(direction)
	}
	return c.request(ctx, http.MethodDelete, path, nil, nil)
}

// Exec runs a predefined command on the origin and streams its output to the
// given writers. The rendered command line is written to notice. It returns the
// command's exit code.
func (c *Client) Exec(ctx context.Context, req api.ExecRequest, notice, stdout, stderr io.Writer) (int, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return -1, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/sessions/"+c.sessionID+"/exec", bytes.NewReader(data))
	if err != nil {
		return -1, err
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return -1, decodeError(resp)
	}

	decoder := json.NewDecoder(resp.Body)
	code := 0
	for {
		var frame api.ExecFrame
		if err := decoder.Decode(&frame); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return -1, err
		}
		switch {
		case frame.Error != "":
			return -1, errors.New(frame.Error)
		case frame.Command != "":
			if notice != nil {
				fmt.Fprintf(notice, "Executing `%s` ...\n", frame.Command)
			}
		case frame.ExitCode != nil:
			code = *frame.ExitCode
		case frame.Stream == "stderr":
			_, _ = io.WriteString(stderr, frame.Data)
		default:
			_, _ = io.WriteString(stdout, frame.Data)
		}
	}
	return code, nil
}

func (c *Client) ListCommands(ctx context.Context) ([]api.CommandResponse, error) {
	var commands []api.CommandResponse
	err := c.request(ctx, http.MethodGet, "/commands", nil, &commands)
	return commands, err
}

func (c *Client) AddCommand(ctx context.Context, name, command, alias string, disabled bool) (api.CommandResponse, error) {
	body := api.CreateCommandRequest{Name: name, Command: command, Alias: alias, Disabled: disabled}
	var created api.CommandResponse
	err := c.request(ctx, http.MethodPost, "/commands", body, &created)
	return created, err
}

func (c *Client) UpdateCommand(ctx context.Context, name string, req api.UpdateCommandRequest) (api.CommandResponse, error) {
	var updated api.CommandResponse
	err := c.request(ctx, http.MethodPut, "/commands/"+name, req, &updated)
	return updated, err
}

func (c *Client) RemoveCommand(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodDelete, "/commands/"+name, nil, nil)
}

func (c *Client) SetCommandDisabled(ctx context.Context, name string, disabled bool) error {
	action := "enable"
	if disabled {
		action = "disable"
	}
	return c.request(ctx, http.MethodPost, "/commands/"+name+"/"+action, nil, nil)
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return decodeError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeError(resp *http.Response) error {
	var payload api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && payload.Error != "" {
		return fmt.Errorf("%s", payload.Error)
	}
	return fmt.Errorf("request failed with status %d", resp.StatusCode)
}
