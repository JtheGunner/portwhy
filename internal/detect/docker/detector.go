package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

const shortIDLength = 12

// Detector explains a socket whose port a running container publishes. It
// matches on the published port, not on the owning process: under Docker
// Desktop, Colima and OrbStack the listener is a VM helper. The container list
// is fetched at most once per Detector; when the daemon is unreachable the
// Detector never matches, so the next detector in the chain takes over.
type Detector struct {
	api API
	now func() time.Time

	once       sync.Once
	containers []Container
}

// New returns a Detector that asks api for the container list.
func New(api API) *Detector {
	return &Detector{api: api, now: time.Now}
}

// Detect implements detect.Detector.
func (d *Detector) Detect(ctx context.Context, s socket.Socket, _ *proc.Process) (*detect.Explanation, bool) {
	container, port, ok := d.find(ctx, s)
	if !ok {
		return nil, false
	}

	name := containerName(container)
	cleanName := textutil.Clean(name)
	e := &detect.Explanation{
		Source:     detect.SourceDocker,
		Title:      fmt.Sprintf("Docker container %q (%s)", cleanName, textutil.Clean(container.Image)),
		Details:    []detect.Detail{{Key: "Mapping", Value: fmt.Sprintf("%d→%d", port.PublicPort, port.PrivatePort)}},
		Terminator: &terminator{api: d.api, id: container.ID, name: name},
	}

	// The inspect call is best effort: without it the container is still
	// explained and stoppable, only uptime and restart policy are missing.
	if in, err := d.api.InspectContainer(ctx, container.ID); err == nil {
		if started := in.State.StartedAt; !started.IsZero() {
			e.Details = append(e.Details, detect.Detail{Key: "Uptime", Value: detect.FormatUptime(d.now().Sub(started))})
		}
		policy := in.HostConfig.RestartPolicy.Name
		if policy != "" && policy != "no" {
			e.Details = append(e.Details, detect.Detail{Key: "Restart policy", Value: textutil.Clean(policy)})
		}
		if policy == "always" || policy == "unless-stopped" {
			e.Hints = append(e.Hints, fmt.Sprintf("restart policy %q: the container comes back after a Docker restart", policy))
		}
	}
	return e, true
}

// find returns the container publishing the socket's port and protocol.
func (d *Detector) find(ctx context.Context, s socket.Socket) (Container, Port, bool) {
	d.once.Do(func() {
		// A failed fetch leaves the list empty and is not retried: every port
		// of the run then falls through to the next detector at once.
		if cs, err := d.api.ListContainers(ctx); err == nil {
			d.containers = cs
		}
	})
	for _, c := range d.containers {
		for _, p := range c.Ports {
			if p.PublicPort == s.Port && strings.EqualFold(p.Type, string(s.Protocol)) {
				return c, p, true
			}
		}
	}
	return Container{}, Port{}, false
}

// containerName returns the container's first name without the leading slash,
// or its short id when it has none.
func containerName(c Container) string {
	for _, n := range c.Names {
		if name := strings.TrimPrefix(n, "/"); name != "" {
			return name
		}
	}
	if len(c.ID) > shortIDLength {
		return c.ID[:shortIDLength]
	}
	return c.ID
}

// terminator stops a container through the Docker API.
type terminator struct {
	api  API
	id   string
	name string
}

var _ kill.Terminator = (*terminator)(nil)

func (t *terminator) Describe() string {
	return "docker stop " + textutil.Clean(t.name)
}

func (t *terminator) Terminate(ctx context.Context) error {
	if err := t.api.StopContainer(ctx, t.id); err != nil {
		return fmt.Errorf("stop container %q: %w", textutil.Clean(t.name), err)
	}
	return nil
}
