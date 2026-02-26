package worker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/musher-dev/mush/internal/client"
)

const (
	// WorkerHeartbeatInterval is the interval for worker heartbeats.
	WorkerHeartbeatInterval = 30 * time.Second
)

// DefaultWorkerInfo returns a name and metadata for worker registration.
func DefaultWorkerInfo() (name string, metadata map[string]any) {
	name, _ = os.Hostname()
	if name == "" {
		name = "unknown-host"
	}

	metadata = map[string]any{
		"hostname": name,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}

	return name, metadata
}

// Register registers a new worker and returns its worker ID.
func Register(
	ctx context.Context,
	apiClient *client.Client,
	habitatID string,
	instanceID string,
	name string,
	metadata map[string]any,
	version string,
) (string, error) {
	if instanceID == "" {
		instanceID = uuid.NewString()
	}

	req := &client.RegisterWorkerRequest{
		InstanceID:     instanceID,
		HabitatID:      habitatID,
		Name:           name,
		WorkerType:     "harness",
		ClientVersion:  version,
		ClientMetadata: metadata,
	}

	resp, err := apiClient.RegisterWorker(ctx, req)
	if err != nil {
		return "", fmt.Errorf("register worker: %w", err)
	}

	if resp.WorkerID == "" {
		return "", fmt.Errorf("register returned empty worker ID")
	}

	return resp.WorkerID, nil
}

// StartHeartbeat sends periodic worker heartbeats until the context is canceled.
// If onError is non-nil, it is called whenever a heartbeat attempt fails.
func StartHeartbeat(
	ctx context.Context,
	apiClient *client.Client,
	workerID string,
	currentJobID func() string,
	onError func(error),
) {
	if workerID == "" {
		return
	}

	ticker := time.NewTicker(WorkerHeartbeatInterval)

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

				if _, err := apiClient.HeartbeatWorker(ctx, workerID, jobID); err != nil {
					if onError != nil {
						onError(err)
					}
				}
			}
		}
	}()
}

// Deregister gracefully disconnects a worker.
func Deregister(apiClient *client.Client, workerID string, completed, failed int) error {
	if workerID == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := client.DeregisterWorkerRequest{
		Reason:        "graceful_shutdown",
		JobsCompleted: completed,
		JobsFailed:    failed,
	}

	if err := apiClient.DeregisterWorker(ctx, workerID, req); err != nil {
		return fmt.Errorf("deregister worker %s: %w", workerID, err)
	}

	return nil
}
