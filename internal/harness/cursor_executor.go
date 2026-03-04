//go:build unix

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
)

// CursorExecutor runs jobs via Cursor Agent CLI.
// Each queued job runs as a one-shot process. Bundle mode starts an interactive PTY session.
type CursorExecutor struct {
	opts SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	pgid       int
	waitDoneCh chan struct{}

	mcpConfigSig     string
	mcpConfigContent string

	interactiveConfigCleanup func() error
}

func init() {
	Register(Info{
		Name:      "cursor",
		Available: AvailableFunc("cursor"),
		New:       func() Executor { return &CursorExecutor{} },
		MCPSpec: &MCPSpec{
			Def:         mustGetProvider("cursor").MCP,
			BuildConfig: BuildCursorMCPConfig,
		},
	})
}

// Setup stores options and starts interactive mode for bundle sessions.
func (e *CursorExecutor) Setup(ctx context.Context, opts *SetupOptions) error {
	e.opts = *opts

	if _, err := exec.LookPath("cursor-agent"); err != nil {
		return fmt.Errorf("cursor-agent CLI not found in PATH")
	}

	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			return err
		}
	}

	if opts.BundleDir != "" {
		if err := e.startInteractive(ctx, opts); err != nil {
			e.cleanupInteractiveConfig()
			return err
		}
	}

	if opts.OnReady != nil {
		opts.OnReady()
	}

	return nil
}

// Execute runs a one-shot cursor-agent command and returns normalized output.
func (e *CursorExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &ExecError{
			Reason:  "execution_error",
			Message: "cursor interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := getPromptFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	workDir := cursorWorkDirFromJob(job)

	args := []string{"--print", "--output-format", "text", prompt}
	if workDir != "" {
		args = append([]string{"-C", workDir}, args...)
	}

	cmd := exec.CommandContext(ctx, "cursor-agent", args...) //nolint:gosec // G204: command originates from trusted job execution payload

	cmd.Env = os.Environ()

	if job.Execution != nil {
		for k, v := range job.Execution.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	cmd.Env = append(cmd.Env,
		fmt.Sprintf("MUSH_JOB_ID=%s", job.ID),
		fmt.Sprintf("MUSH_JOB_NAME=%s", job.GetDisplayName()),
		fmt.Sprintf("MUSH_JOB_QUEUE=%s", job.QueueID),
	)

	cleanup, env, err := buildCursorConfigEnv(e.mcpConfigContent, workDir)
	if err != nil {
		return nil, &ExecError{Reason: "execution_error", Message: err.Error()}
	}

	if cleanup != nil {
		defer func() {
			_ = cleanup()
		}()
	}

	cmd.Env = append(cmd.Env, env...)

	var output strings.Builder

	outWriter := io.Writer(&output)
	if e.opts.TermWriter != nil {
		outWriter = io.MultiWriter(e.opts.TermWriter, &output)
	}

	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	if runErr != nil {
		return nil, handleOneShotRunError(ctx, runErr, output.String(), "cursor-agent")
	}

	resultOutput := ansi.Strip(strings.TrimSpace(output.String()))

	return &ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     resultOutput,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

// Reset is a no-op for cursor one-shot worker jobs.
func (e *CursorExecutor) Reset(_ context.Context) error {
	return nil
}

// Resize implements Resizable for interactive bundle sessions.
func (e *CursorExecutor) Resize(rows, cols int) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return
	}

	_ = pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// WriteInput forwards terminal input to the interactive Cursor process.
func (e *CursorExecutor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to cursor pty: %w", err)
	}

	return n, nil
}

// Interrupt implements InterruptHandler for interactive bundle sessions.
func (e *CursorExecutor) Interrupt() error {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return nil
	}

	_, err := ptmx.Write([]byte{ctrlC})
	if err != nil {
		return fmt.Errorf("interrupt cursor pty: %w", err)
	}

	return nil
}

// NeedsRefresh implements Refreshable.
func (e *CursorExecutor) NeedsRefresh(cfg *client.RunnerConfigResponse) bool {
	specs := BuildMCPProviderSpecs(cfg, time.Now())

	sig, err := MCPSignature(specs)
	if err != nil {
		return true
	}

	return sig != e.mcpConfigSig
}

// ApplyRefresh implements Refreshable.
func (e *CursorExecutor) ApplyRefresh(_ context.Context, cfg *client.RunnerConfigResponse) error {
	return e.applyRunnerConfig(cfg)
}

func (e *CursorExecutor) applyRunnerConfig(cfg *client.RunnerConfigResponse) error {
	now := time.Now()

	info, ok := Lookup("cursor")
	if !ok || info.MCPSpec == nil {
		return nil
	}

	specs := BuildMCPProviderSpecs(cfg, now)

	sig, err := MCPSignature(specs)
	if err != nil {
		return err
	}

	content, err := info.MCPSpec.BuildConfig(specs)
	if err != nil {
		return err
	}

	e.mcpConfigSig = sig
	e.mcpConfigContent = string(content)

	return nil
}

func (e *CursorExecutor) startInteractive(ctx context.Context, opts *SetupOptions) error {
	args := []string{}
	if opts.BundleDir != "" {
		args = append(args, "-C", opts.BundleDir)
	}

	cmd := exec.CommandContext(ctx, "cursor-agent", args...) //nolint:gosec // G204: args from controlled input

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	cleanup, env, err := buildCursorConfigEnv(e.mcpConfigContent, opts.BundleDir)
	if err != nil {
		return err
	}

	cmd.Env = append(cmd.Env, env...)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		if cleanup != nil {
			_ = cleanup()
		}

		return fmt.Errorf("start cursor interactive session: %w", err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.ptmx = ptmx
	e.pgid = 0
	e.interactiveConfigCleanup = cleanup

	if cmd.Process != nil && cmd.Process.Pid > 0 {
		if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil {
			e.pgid = pgid
		}
	}

	e.waitDoneCh = make(chan struct{})
	waitDoneCh := e.waitDoneCh
	e.mu.Unlock()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				if opts.TermWriter != nil {
					_, _ = opts.TermWriter.Write(buf[:n])
				}

				if opts.OnOutput != nil {
					opts.OnOutput(buf[:n])
				}
			}

			if readErr != nil {
				return
			}
		}
	}()

	go func() {
		_ = cmd.Wait()

		close(waitDoneCh)

		e.cleanupInteractiveConfig()

		if opts.OnExit != nil {
			opts.OnExit()
		}
	}()

	return nil
}

// Teardown stops the interactive Cursor process when running in bundle mode.
func (e *CursorExecutor) Teardown() {
	e.mu.Lock()
	cmd := e.cmd
	ptmx := e.ptmx
	pgid := e.pgid
	waitDoneCh := e.waitDoneCh
	e.cmd = nil
	e.ptmx = nil
	e.pgid = 0
	e.waitDoneCh = nil
	e.mu.Unlock()

	stopInteractiveProcess(cmd, ptmx, pgid, waitDoneCh)

	e.cleanupInteractiveConfig()
}

func (e *CursorExecutor) cleanupInteractiveConfig() {
	e.mu.Lock()
	cleanup := e.interactiveConfigCleanup
	e.interactiveConfigCleanup = nil
	e.mu.Unlock()

	if cleanup != nil {
		_ = cleanup()
	}
}

func buildCursorConfigEnv(configContent, projectDir string) (cleanup func() error, env []string, err error) {
	if strings.TrimSpace(configContent) == "" {
		return nil, nil, nil
	}

	mergedContent, err := mergeCursorConfig(configContent, projectDir)
	if err != nil {
		return nil, nil, err
	}

	configFile, err := os.CreateTemp("", "mush-cursor-agent-config-*.json")
	if err != nil {
		return nil, nil, fmt.Errorf("create cursor config file: %w", err)
	}

	configPath := configFile.Name()

	if _, writeErr := configFile.Write(mergedContent); writeErr != nil {
		_ = configFile.Close()
		_ = os.Remove(configPath)

		return nil, nil, fmt.Errorf("write cursor config file: %w", writeErr)
	}

	if closeErr := configFile.Close(); closeErr != nil {
		_ = os.Remove(configPath)
		return nil, nil, fmt.Errorf("close cursor config file: %w", closeErr)
	}

	return func() error { return os.Remove(configPath) },
		[]string{fmt.Sprintf("CUA_CONFIG_PATH=%s", configPath)},
		nil
}

func mergeCursorConfig(generatedContent, projectDir string) ([]byte, error) {
	var generatedConfig map[string]any
	if err := json.Unmarshal([]byte(generatedContent), &generatedConfig); err != nil {
		return nil, fmt.Errorf("decode generated cursor config: %w", err)
	}

	baseConfig := map[string]any{}

	if basePath := resolveCursorBaseConfigPath(projectDir); basePath != "" {
		if data, err := os.ReadFile(filepath.Clean(basePath)); err == nil && strings.TrimSpace(string(data)) != "" {
			_ = json.Unmarshal(data, &baseConfig)
		}
	}

	if mcpServers, ok := generatedConfig["mcpServers"]; ok {
		baseConfig["mcpServers"] = mcpServers
	}

	mergedContent, err := json.MarshalIndent(baseConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged cursor config: %w", err)
	}

	return mergedContent, nil
}

func resolveCursorBaseConfigPath(projectDir string) string {
	if envPath := strings.TrimSpace(os.Getenv("CUA_CONFIG_PATH")); envPath != "" {
		if fileExists(envPath) {
			return envPath
		}
	}

	if strings.TrimSpace(projectDir) != "" {
		projectPath := filepath.Join(projectDir, ".cursor", "agent.json")
		if fileExists(projectPath) {
			return projectPath
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return ""
	}

	userPath := filepath.Join(homeDir, ".cursor", "agent.json")
	if fileExists(userPath) {
		return userPath
	}

	return ""
}

func cursorWorkDirFromJob(job *client.Job) string {
	if job != nil && job.Execution != nil && strings.TrimSpace(job.Execution.WorkingDirectory) != "" {
		return job.Execution.WorkingDirectory
	}

	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}

	return workDir
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

var (
	_ Executor         = (*CursorExecutor)(nil)
	_ InputReceiver    = (*CursorExecutor)(nil)
	_ Resizable        = (*CursorExecutor)(nil)
	_ InterruptHandler = (*CursorExecutor)(nil)
	_ Refreshable      = (*CursorExecutor)(nil)
)
