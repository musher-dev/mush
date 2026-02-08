package linking

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/output"
)

const (
	// LinkHeartbeatInterval is the interval for link heartbeats.
	LinkHeartbeatInterval = 30 * time.Second
)

// DefaultLinkInfo returns a name and metadata for link registration.
func DefaultLinkInfo() (name string, metadata map[string]interface{}) {
	name, _ = os.Hostname()
	if name == "" {
		name = "unknown-host"
	}

	metadata = map[string]interface{}{
		"hostname": name,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}

	return name, metadata
}

// Register registers a new link and returns its link ID.
func Register(
	ctx context.Context,
	c *client.Client,
	habitatID string,
	instanceID string,
	name string,
	metadata map[string]interface{},
	version string,
) (string, error) {
	if instanceID == "" {
		instanceID = uuid.NewString()
	}

	req := &client.RegisterLinkRequest{
		InstanceID:     instanceID,
		HabitatID:      habitatID,
		Name:           name,
		LinkType:       "agent",
		ClientVersion:  version,
		ClientMetadata: metadata,
	}

	resp, err := c.RegisterLink(ctx, req)
	if err != nil {
		return "", err
	}

	if resp.LinkID == "" {
		return "", fmt.Errorf("register returned empty link ID")
	}

	return resp.LinkID, nil
}

// StartHeartbeat sends periodic link heartbeats until the context is cancelled.
func StartHeartbeat(ctx context.Context, c *client.Client, linkID string, currentJobID func() string, out *output.Writer) {
	if linkID == "" {
		return
	}

	ticker := time.NewTicker(LinkHeartbeatInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var jobID string
				if currentJobID != nil {
					jobID = currentJobID()
				}
				if _, err := c.HeartbeatLink(ctx, linkID, jobID); err != nil {
					out.Debug("Link heartbeat failed: %v", err)
				}
			}
		}
	}()
}

// Deregister gracefully disconnects a link.
func Deregister(c *client.Client, linkID string, completed, failed int, out *output.Writer) {
	if linkID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := client.DeregisterLinkRequest{
		Reason:        "graceful_shutdown",
		JobsCompleted: completed,
		JobsFailed:    failed,
	}

	if err := c.DeregisterLink(ctx, linkID, req); err != nil {
		out.Warning("Failed to deregister link: %v", err)
	} else {
		out.Print("Link deregistered: %s\n", linkID)
	}
}
