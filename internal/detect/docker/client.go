package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/JtheGunner/portwhy/internal/textutil"
)

const (
	// defaultReadTimeout bounds every read request, so an unreachable or
	// hanging daemon never delays the answer noticeably.
	defaultReadTimeout = 2 * time.Second
	// defaultStopTimeout leaves room for the daemon's own grace period (10 s by
	// default) before it kills the container.
	defaultStopTimeout = 30 * time.Second
	maxBodyBytes       = 4 << 20
	maxErrorBytes      = 4 << 10
)

// Port is one port entry of a container.
type Port struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Container is the part of a GET /containers/json entry that portwhy uses.
type Container struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	Ports []Port   `json:"Ports"`
}

// Inspect is the part of GET /containers/<id>/json that portwhy uses.
type Inspect struct {
	State struct {
		StartedAt time.Time `json:"StartedAt"`
	} `json:"State"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
}

// API is what the detector needs from the Docker daemon.
type API interface {
	ListContainers(ctx context.Context) ([]Container, error)
	InspectContainer(ctx context.Context, id string) (Inspect, error)
	StopContainer(ctx context.Context, id string) error
}

// Client talks to the Docker Engine API over a Unix socket.
type Client struct {
	http        *http.Client
	readTimeout time.Duration
	stopTimeout time.Duration
}

// NewClient returns a Client for the daemon listening on socketPath.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}
	return &Client{
		http:        &http.Client{Transport: transport},
		readTimeout: defaultReadTimeout,
		stopTimeout: defaultStopTimeout,
	}
}

// ListContainers returns the running containers.
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	var out []Container
	if err := c.getJSON(ctx, "/containers/json", &out); err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return out, nil
}

// InspectContainer returns the details of one container.
func (c *Client) InspectContainer(ctx context.Context, id string) (Inspect, error) {
	var out Inspect
	if err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", &out); err != nil {
		return Inspect{}, fmt.Errorf("inspect container: %w", err)
	}
	return out, nil
}

// StopContainer stops a container with the daemon's default grace period. A
// container that is already stopped counts as success.
func (c *Client) StopContainer(ctx context.Context, id string) error {
	status, body, err := c.do(ctx, c.stopTimeout, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop")
	if err != nil {
		return err
	}
	if status == http.StatusNoContent || status == http.StatusNotModified {
		return nil
	}
	return statusError(status, body)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	status, body, err := c.do(ctx, c.readTimeout, http.MethodGet, path)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// do performs one request and returns the status and the (size-limited) body.
func (c *Client) do(ctx context.Context, timeout time.Duration, method, path string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("docker API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("read docker API response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// statusError builds an error from a non-success response. The daemon's
// message is external text, so it is cleaned before it can reach a terminal.
func statusError(status int, body []byte) error {
	if len(body) > maxErrorBytes {
		body = body[:maxErrorBytes]
	}
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Message != "" {
		return fmt.Errorf("docker API returned status %d: %s", status, textutil.Clean(payload.Message))
	}
	return fmt.Errorf("docker API returned status %d", status)
}
