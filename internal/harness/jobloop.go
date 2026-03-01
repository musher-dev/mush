//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/observability"
)

// JobLoop manages job polling, execution, heartbeat, and worker lifecycle.
type JobLoop struct {
	client     *client.Client
	cfg        *config.Config
	habitatID  string
	queueID    string
	workerID   string
	instanceID string
	signalDir  string

	// Set once, read-only thereafter.
	executors          map[string]Executor
	supportedHarnesses []string

	// Job lifecycle state (guarded by jobMu).
	jobMu           sync.Mutex
	currentJob      *client.Job
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc

	// Status state (guarded by statusMu).
	statusMu      sync.Mutex
	status        ConnectionStatus
	lastHeartbeat time.Time
	completed     int
	failed        int
	lastError     string
	lastErrorTime time.Time

	// Runner config refresh state (guarded by refreshMu).
	refreshMu       sync.Mutex
	refreshInterval time.Duration
	runnerConfig    *client.RunnerConfigResponse

	// Callbacks wired by RootModel.
	drawStatusBar func()
	infof         func(format string, args ...any)
	signalDone    func()
	now           func() time.Time
}

// JobLoopSnapshot holds a point-in-time snapshot of job loop state.
type JobLoopSnapshot struct {
	StatusLabel   string
	LastHeartbeat time.Time
	Completed     int
	Failed        int
	LastError     string
	LastErrorTime time.Time
	JobID         string
}

// Snapshot returns a consistent snapshot of the job loop state.
func (jl *JobLoop) Snapshot() JobLoopSnapshot {
	jl.statusMu.Lock()
	snap := JobLoopSnapshot{
		StatusLabel:   jl.status.String(),
		LastHeartbeat: jl.lastHeartbeat,
		Completed:     jl.completed,
		Failed:        jl.failed,
		LastError:     jl.lastError,
		LastErrorTime: jl.lastErrorTime,
	}
	jl.statusMu.Unlock()

	jl.jobMu.Lock()
	if jl.currentJob != nil {
		snap.JobID = jl.currentJob.ID
	}
	jl.jobMu.Unlock()

	return snap
}

// RunnerConfig returns the current runner config.
func (jl *JobLoop) RunnerConfig() *client.RunnerConfigResponse {
	jl.refreshMu.Lock()
	defer jl.refreshMu.Unlock()

	return jl.runnerConfig
}

// CurrentJobID returns the ID of the currently executing job, or "".
func (jl *JobLoop) CurrentJobID() string {
	jl.jobMu.Lock()
	defer jl.jobMu.Unlock()

	if jl.currentJob == nil {
		return ""
	}

	return jl.currentJob.ID
}

// HasActiveClaudeJob returns true when a Claude-type job is in progress.
func (jl *JobLoop) HasActiveClaudeJob() bool {
	jl.jobMu.Lock()
	defer jl.jobMu.Unlock()

	if jl.currentJob == nil {
		return false
	}

	return jl.currentJob.GetHarnessType() == "claude"
}

// SetLastError records an error to be displayed in the status bar.
func (jl *JobLoop) SetLastError(msg string) {
	jl.statusMu.Lock()
	jl.lastError = msg
	jl.lastErrorTime = jl.currentTime()
	jl.statusMu.Unlock()
}

// currentTime returns the current time, using the injected clock when available.
func (jl *JobLoop) currentTime() time.Time {
	if jl.now != nil {
		return jl.now()
	}

	return time.Now()
}

// Run executes the job manager loop, polling for and processing jobs.
func (jl *JobLoop) Run(ctx context.Context, done <-chan struct{}) {
	// Wait for Claude to be ready if it's a supported harness.
	jl.statusMu.Lock()
	jl.status = StatusConnected
	jl.statusMu.Unlock()

	pollInterval := jl.cfg.PollInterval()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		default:
		}

		// Check if any Refreshable executors need restart.
		if err := jl.maybeRefreshExecutors(ctx); err != nil {
			jl.SetLastError(fmt.Sprintf("Executor refresh failed: %v", err))
			time.Sleep(2 * time.Second)

			continue
		}

		// Poll for a job.
		job, err := jl.client.ClaimJob(ctx, jl.habitatID, jl.queueID, int(pollInterval.Seconds()))
		if err != nil {
			if ctx.Err() != nil {
				return // Context canceled
			}

			jl.SetLastError(fmt.Sprintf("Claim failed: %v", err))
			time.Sleep(5 * time.Second) // Backoff on error

			continue
		}

		if job == nil {
			continue // No job, poll again
		}

		// Map execution.harnessType to local harness selection.
		harnessType := job.GetHarnessType()
		if harnessType == "" {
			jl.SetLastError("Missing harness type in job execution config")
			jl.releaseJob(ctx, job)

			continue
		}

		if !jl.isHarnessSupported(harnessType) {
			errMsg := fmt.Sprintf("Unsupported harness type: %s", harnessType)
			jl.SetLastError(errMsg)
			jl.releaseJob(ctx, job)

			continue
		}

		// Process the job.
		jl.processJob(ctx, job)
	}
}

// processJob handles the lifecycle of a single job using the executor.
func (jl *JobLoop) processJob(parentCtx context.Context, job *client.Job) {
	ctx, span := observability.Tracer("mush.harness").Start(parentCtx, "job.process",
		trace.WithAttributes(
			attribute.String("job.id", job.ID),
			attribute.String("job.queue_id", job.QueueID),
			attribute.String("job.harness_type", job.GetHarnessType()),
			attribute.String("job.priority", job.Priority),
			attribute.Int("job.attempt_number", job.AttemptNumber),
		),
	)
	defer span.End()

	harnessType := job.GetHarnessType()

	executor, ok := jl.executors[harnessType]
	if !ok {
		jl.SetLastError(fmt.Sprintf("No executor for harness type: %s", harnessType))
		span.SetStatus(codes.Error, "unsupported harness type")
		jl.releaseJob(ctx, job)

		return
	}

	jl.jobMu.Lock()
	jl.currentJob = job
	jl.jobMu.Unlock()

	// Update status bar
	jl.statusMu.Lock()
	jl.status = StatusProcessing
	jl.statusMu.Unlock()
	jl.drawStatusBar()

	// Start heartbeat for the job.
	jl.heartbeatCtx, jl.heartbeatCancel = context.WithCancel(parentCtx)
	go jl.heartbeatLoop(jl.heartbeatCtx, job.ID)

	defer func() {
		jl.heartbeatCancel()
		jl.jobMu.Lock()
		jl.currentJob = nil
		jl.jobMu.Unlock()
		jl.statusMu.Lock()
		jl.status = StatusConnected
		jl.statusMu.Unlock()
	}()

	if _, err := jl.client.StartJob(ctx, job.ID); err != nil {
		jl.SetLastError(fmt.Sprintf("Start job failed: %v", err))
	}

	// Determine execution timeout.
	execTimeout := DefaultExecutionTimeout
	if job.Execution != nil && job.Execution.TimeoutMs > 0 {
		execTimeout = time.Duration(job.Execution.TimeoutMs) * time.Millisecond
	}

	execCtx, cancelExec := context.WithTimeout(ctx, execTimeout)
	defer cancelExec()

	// Execute the job via the executor.
	execCtx, execSpan := observability.Tracer("mush.harness").Start(execCtx, "job.execute",
		trace.WithAttributes(
			attribute.String("job.id", job.ID),
			attribute.String("job.harness_type", harnessType),
		),
	)

	result, execErr := executor.Execute(execCtx, job)

	execSpan.End()

	if execErr != nil {
		reason := "execution_error"
		msg := execErr.Error()
		retry := true

		var ee *ExecError
		if errors.As(execErr, &ee) {
			reason = ee.Reason
			msg = ee.Message
			retry = ee.Retry
		}

		span.RecordError(execErr)
		span.SetStatus(codes.Error, reason)

		if retry {
			jl.failJob(ctx, job, reason, msg)
		} else {
			jl.failJobNoRetry(ctx, job, reason, msg)
		}

		return
	}

	span.SetStatus(codes.Ok, "")
	jl.completeJob(ctx, job, result.OutputData)

	// Reset the executor for the next job.
	if err := executor.Reset(parentCtx); err != nil {
		jl.SetLastError(fmt.Sprintf("Executor reset failed: %v", err))
	}
}

// heartbeatLoop sends periodic heartbeats for the current job.
func (jl *JobLoop) heartbeatLoop(ctx context.Context, jobID string) {
	interval := jl.cfg.HeartbeatInterval()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := jl.client.HeartbeatJob(ctx, jobID)
			if err != nil {
				jl.SetLastError(fmt.Sprintf("Heartbeat failed: %v", err))
				continue
			}

			jl.statusMu.Lock()
			jl.lastHeartbeat = time.Now()
			jl.statusMu.Unlock()
		}
	}
}

// completeJob reports job completion to the API.
func (jl *JobLoop) completeJob(ctx context.Context, job *client.Job, outputData map[string]any) {
	err := jl.client.CompleteJob(ctx, job.ID, outputData)
	if err != nil {
		jl.SetLastError(fmt.Sprintf("Complete failed: %v", err))
		jl.failJob(ctx, job, "completion_report_failed", err.Error())

		return
	}

	jl.statusMu.Lock()
	jl.completed++
	jl.statusMu.Unlock()
}

// releaseJob returns a job to the queue.
func (jl *JobLoop) releaseJob(ctx context.Context, job *client.Job) {
	if err := jl.client.ReleaseJob(ctx, job.ID); err != nil {
		jl.SetLastError(fmt.Sprintf("Release failed: %v", err))
	}
}

// failJob reports job failure to the API (retryable).
func (jl *JobLoop) failJob(ctx context.Context, job *client.Job, reason, message string) {
	err := jl.client.FailJob(ctx, job.ID, reason, message, true)
	if err != nil {
		jl.SetLastError(fmt.Sprintf("Fail report failed: %v", err))
	}

	jl.statusMu.Lock()
	jl.failed++
	jl.statusMu.Unlock()
}

// failJobNoRetry reports a permanent job failure (no retry).
func (jl *JobLoop) failJobNoRetry(ctx context.Context, job *client.Job, reason, message string) {
	err := jl.client.FailJob(ctx, job.ID, reason, message, false)
	if err != nil {
		jl.SetLastError(fmt.Sprintf("Fail report failed: %v", err))
	}

	jl.statusMu.Lock()
	jl.failed++
	jl.statusMu.Unlock()
}

// RunnerConfigRefreshLoop periodically refreshes the runner config for MCP credential rotation.
func (jl *JobLoop) RunnerConfigRefreshLoop(ctx context.Context, done <-chan struct{}) {
	interval := jl.refreshInterval
	if interval <= 0 {
		interval = normalizeRefreshInterval(0)
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-timer.C:
			cfg, err := jl.client.GetRunnerConfig(ctx)
			if err != nil {
				jl.SetLastError(fmt.Sprintf("Runner config refresh failed: %v", err))
				timer.Reset(interval)

				continue
			}

			jl.refreshMu.Lock()
			interval = normalizeRefreshInterval(cfg.RefreshAfterSeconds)
			jl.refreshInterval = interval

			// Check all refreshable executors.
			for _, executor := range jl.executors {
				if r, ok := executor.(Refreshable); ok {
					if r.NeedsRefresh(cfg) {
						jl.runnerConfig = cfg
					}
				}
			}

			jl.refreshMu.Unlock()
			timer.Reset(interval)
		}
	}
}

func (jl *JobLoop) maybeRefreshExecutors(ctx context.Context) error {
	if jl.CurrentJobID() != "" {
		return nil
	}

	jl.refreshMu.Lock()
	cfg := jl.runnerConfig
	jl.refreshMu.Unlock()

	for harnessName, executor := range jl.executors {
		r, ok := executor.(Refreshable)
		if !ok {
			continue
		}

		if !r.NeedsRefresh(cfg) {
			continue
		}

		if err := r.ApplyRefresh(ctx, cfg); err != nil {
			return fmt.Errorf("apply refresh for %s: %w", harnessName, err)
		}
	}

	return nil
}

func (jl *JobLoop) isHarnessSupported(harnessType string) bool {
	for _, a := range jl.supportedHarnesses {
		if a == harnessType {
			return true
		}
	}

	return false
}
