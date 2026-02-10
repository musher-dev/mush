//go:build unix

package install_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath returns the absolute path to install.sh.
func scriptPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatalf("resolving install.sh path: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("install.sh not found at %s: %v", p, err)
	}
	return p
}

// runResult holds the output of a script execution.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runScript runs install.sh with the given arguments and environment overrides.
func runScript(t *testing.T, args, env []string) runResult {
	t.Helper()
	cmdArgs := append([]string{scriptPath(t)}, args...)
	cmd := exec.CommandContext(context.Background(), "sh", cmdArgs...)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running install.sh: %v", err)
		}
	}
	return runResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
	}
}

// runFunction sources install.sh with INSTALL_SH_TESTING=1 and calls the given shell function.
func runFunction(t *testing.T, fnCall string, env []string) runResult {
	t.Helper()
	script := fmt.Sprintf(". '%s'\n%s", scriptPath(t), fnCall)
	cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
	cmd.Env = append(os.Environ(), "INSTALL_SH_TESTING=1")
	cmd.Env = append(cmd.Env, env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running function %q: %v\nstderr: %s", fnCall, err, stderr.String())
		}
	}
	return runResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
	}
}

// makeFakeArchive creates a tar.gz containing a fake mush binary.
// Returns the archive bytes and its SHA-256 hex digest.
func makeFakeArchive(t *testing.T) (archiveData []byte, checksumHex string) {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := []byte("#!/bin/sh\necho \"mush v0.0.0-test\"\n")
	hdr := &tar.Header{
		Name: "mush",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}

	archiveBytes := buf.Bytes()
	sum := sha256.Sum256(archiveBytes)
	return archiveBytes, fmt.Sprintf("%x", sum)
}

// newMockGitHub creates an httptest server that mimics GitHub releases.
func newMockGitHub(t *testing.T, version string, archiveBytes []byte, checksumContent string) *httptest.Server {
	t.Helper()
	tag := "v" + version
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/releases/latest":
			http.Redirect(w, r, fmt.Sprintf("/releases/tag/%s", tag), http.StatusFound)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(archiveBytes)
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksumContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

// fakeUname creates a fake uname script in a temp directory and returns the dir path.
func fakeUname(t *testing.T, output string) string {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", output)
	unamePath := filepath.Join(binDir, "uname")
	if err := os.WriteFile(unamePath, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake uname: %v", err)
	}
	return binDir
}

// hostArchiveName returns the archive name the install script would construct
// on the current host for a given version.
func hostArchiveName(t *testing.T, version string) string {
	t.Helper()
	var osName string
	switch runtime.GOOS {
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "darwin"
	default:
		t.Skipf("unsupported OS for install tests: %s", runtime.GOOS)
	}

	var archName string
	switch runtime.GOARCH {
	case "amd64":
		archName = "amd64"
	case "arm64":
		archName = "arm64"
	default:
		t.Skipf("unsupported arch for install tests: %s", runtime.GOARCH)
	}

	return fmt.Sprintf("mush_%s_%s_%s.tar.gz", version, osName, archName)
}

// ── Group 1: Argument parsing ──────────────────────────────────────────────

func TestArgumentParsing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "help flag",
			args:       []string{"--help"},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "help short flag",
			args:       []string{"-h"},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "version without value",
			args:       []string{"--version"},
			wantExit:   1,
			wantStderr: "--version requires a value",
		},
		{
			name:       "prefix without value",
			args:       []string{"--prefix"},
			wantExit:   1,
			wantStderr: "--prefix requires a value",
		},
		{
			name:       "unknown option",
			args:       []string{"--bogus"},
			wantExit:   1,
			wantStderr: "Unknown option",
		},
		{
			name:       "yes flag accepted",
			args:       []string{"--yes", "--help"},
			wantExit:   0,
			wantStdout: "Usage:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runScript(t, tt.args, nil)
			if result.exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s",
					result.exitCode, tt.wantExit, result.stdout, result.stderr)
			}
			if tt.wantStdout != "" && !strings.Contains(result.stdout, tt.wantStdout) {
				t.Errorf("stdout missing %q\ngot: %s", tt.wantStdout, result.stdout)
			}
			if tt.wantStderr != "" && !strings.Contains(result.stderr, tt.wantStderr) {
				t.Errorf("stderr missing %q\ngot: %s", tt.wantStderr, result.stderr)
			}
		})
	}
}

// ── Group 2: Platform detection ────────────────────────────────────────────

func TestDetectOS(t *testing.T) {
	tests := []struct {
		name       string
		uname      string
		wantOutput string
		wantExit   int
		wantStderr string
	}{
		{"linux", "Linux", "linux", 0, ""},
		{"darwin", "Darwin", "darwin", 0, ""},
		{"unsupported", "FreeBSD", "", 1, "Unsupported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := fakeUname(t, tt.uname)
			env := []string{"PATH=" + binDir + ":" + os.Getenv("PATH")}
			result := runFunction(t, "detect_os", env)

			if result.exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstderr: %s", result.exitCode, tt.wantExit, result.stderr)
			}
			if tt.wantOutput != "" && strings.TrimSpace(result.stdout) != tt.wantOutput {
				t.Errorf("output = %q, want %q", strings.TrimSpace(result.stdout), tt.wantOutput)
			}
			if tt.wantStderr != "" && !strings.Contains(result.stderr, tt.wantStderr) {
				t.Errorf("stderr missing %q\ngot: %s", tt.wantStderr, result.stderr)
			}
		})
	}
}

func TestDetectArch(t *testing.T) {
	tests := []struct {
		name       string
		uname      string
		wantOutput string
		wantExit   int
		wantStderr string
	}{
		{"amd64", "x86_64", "amd64", 0, ""},
		{"arm64", "aarch64", "arm64", 0, ""},
		{"unsupported", "i686", "", 1, "Unsupported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := fakeUname(t, tt.uname)
			env := []string{"PATH=" + binDir + ":" + os.Getenv("PATH")}
			result := runFunction(t, "detect_arch", env)

			if result.exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstderr: %s", result.exitCode, tt.wantExit, result.stderr)
			}
			if tt.wantOutput != "" && strings.TrimSpace(result.stdout) != tt.wantOutput {
				t.Errorf("output = %q, want %q", strings.TrimSpace(result.stdout), tt.wantOutput)
			}
			if tt.wantStderr != "" && !strings.Contains(result.stderr, tt.wantStderr) {
				t.Errorf("stderr missing %q\ngot: %s", tt.wantStderr, result.stderr)
			}
		})
	}
}

// ── Group 3: Checksum verification ─────────────────────────────────────────

func TestVerifyChecksum(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		checksumHex string // override; empty means use correct checksum
		archiveName string
		wantExit    int
		wantStderr  string
	}{
		{
			name:        "valid checksum",
			fileContent: "hello world",
			archiveName: "test.tar.gz",
			wantExit:    0,
		},
		{
			name:        "mismatched checksum",
			fileContent: "hello world",
			checksumHex: "0000000000000000000000000000000000000000000000000000000000000000",
			archiveName: "test.tar.gz",
			wantExit:    1,
			wantStderr:  "Checksum mismatch",
		},
		{
			name:        "archive not in checksums",
			fileContent: "hello world",
			archiveName: "missing.tar.gz",
			wantExit:    1,
			wantStderr:  "not found in checksums",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			filePath := filepath.Join(tmpDir, "test.tar.gz")
			if err := os.WriteFile(filePath, []byte(tt.fileContent), 0o644); err != nil {
				t.Fatalf("writing file: %v", err)
			}

			sum := sha256.Sum256([]byte(tt.fileContent))
			actualHex := fmt.Sprintf("%x", sum)

			checksumHex := tt.checksumHex
			if checksumHex == "" {
				checksumHex = actualHex
			}

			checksumFile := filepath.Join(tmpDir, "checksums.txt")
			checksumContent := fmt.Sprintf("%s  test.tar.gz\n", checksumHex)
			if err := os.WriteFile(checksumFile, []byte(checksumContent), 0o644); err != nil {
				t.Fatalf("writing checksums: %v", err)
			}

			fnCall := fmt.Sprintf("verify_checksum %q %q %q", filePath, checksumFile, tt.archiveName)
			result := runFunction(t, fnCall, nil)

			if result.exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstderr: %s", result.exitCode, tt.wantExit, result.stderr)
			}
			if tt.wantStderr != "" && !strings.Contains(result.stderr, tt.wantStderr) {
				t.Errorf("stderr missing %q\ngot: %s", tt.wantStderr, result.stderr)
			}
		})
	}
}

// ── Group 4: maybe_sudo ────────────────────────────────────────────────────

func TestMaybeSudo(t *testing.T) {
	t.Run("writable directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := runFunction(t, fmt.Sprintf("maybe_sudo %q", tmpDir), nil)

		if result.exitCode != 0 {
			t.Errorf("exit code = %d, want 0\nstderr: %s", result.exitCode, result.stderr)
		}
		if strings.TrimSpace(result.stdout) != "" {
			t.Errorf("stdout = %q, want empty (no sudo needed)", result.stdout)
		}
	})

	t.Run("writable ancestor with non-existent child", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "nonexistent", "deep", "path")
		result := runFunction(t, fmt.Sprintf("maybe_sudo %q", target), nil)

		if result.exitCode != 0 {
			t.Errorf("exit code = %d, want 0\nstderr: %s", result.exitCode, result.stderr)
		}
		if strings.TrimSpace(result.stdout) != "" {
			t.Errorf("stdout = %q, want empty (no sudo needed)", result.stdout)
		}
	})
}

// ── Group 5: check_path ────────────────────────────────────────────────────

func TestCheckPath(t *testing.T) {
	t.Run("directory in PATH", func(t *testing.T) {
		result := runFunction(t, "check_path /usr/bin", nil)

		if result.exitCode != 0 {
			t.Errorf("exit code = %d, want 0", result.exitCode)
		}
		if strings.Contains(result.stdout, "not in your PATH") {
			t.Error("should not warn about /usr/bin being missing from PATH")
		}
	})

	t.Run("directory not in PATH", func(t *testing.T) {
		result := runFunction(t, "check_path /some/fake/dir/not/in/path", nil)

		if !strings.Contains(result.stdout, "not in your PATH") {
			t.Errorf("stdout missing PATH warning\ngot: %s", result.stdout)
		}
	})
}

// ── Group 6: End-to-end with mock GitHub ───────────────────────────────────

func TestEndToEnd(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		archiveBytes, checksum := makeFakeArchive(t)
		archiveName := hostArchiveName(t, "0.1.0")
		checksumContent := fmt.Sprintf("%s  %s\n", checksum, archiveName)

		server := newMockGitHub(t, "0.1.0", archiveBytes, checksumContent)
		defer server.Close()

		prefix := t.TempDir()
		result := runScript(t, []string{"--version", "0.1.0", "--prefix", prefix, "--yes"}, []string{
			"MUSH_INSTALL_BASE_URL=" + server.URL,
			"MUSH_INSTALL_INSECURE=1",
		})

		if result.exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s",
				result.exitCode, result.stdout, result.stderr)
		}
		if !strings.Contains(result.stdout, "Successfully installed") {
			t.Errorf("stdout missing success message\ngot: %s", result.stdout)
		}

		binPath := filepath.Join(prefix, "bin", "mush")
		info, err := os.Stat(binPath)
		if err != nil {
			t.Fatalf("binary not found at %s: %v", binPath, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("binary is not executable: mode %o", info.Mode())
		}
	})

	t.Run("checksum failure", func(t *testing.T) {
		archiveBytes, _ := makeFakeArchive(t)
		archiveName := hostArchiveName(t, "0.1.0")
		checksumContent := fmt.Sprintf("%s  %s\n",
			"0000000000000000000000000000000000000000000000000000000000000000", archiveName)

		server := newMockGitHub(t, "0.1.0", archiveBytes, checksumContent)
		defer server.Close()

		prefix := t.TempDir()
		result := runScript(t, []string{"--version", "0.1.0", "--prefix", prefix, "--yes"}, []string{
			"MUSH_INSTALL_BASE_URL=" + server.URL,
			"MUSH_INSTALL_INSECURE=1",
		})

		if result.exitCode == 0 {
			t.Error("expected non-zero exit code for checksum failure")
		}
		if !strings.Contains(result.stderr, "Checksum mismatch") {
			t.Errorf("stderr missing checksum mismatch message\ngot: %s", result.stderr)
		}
	})

	t.Run("download failure 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		prefix := t.TempDir()
		result := runScript(t, []string{"--version", "0.1.0", "--prefix", prefix, "--yes"}, []string{
			"MUSH_INSTALL_BASE_URL=" + server.URL,
			"MUSH_INSTALL_INSECURE=1",
		})

		if result.exitCode == 0 {
			t.Error("expected non-zero exit code for download failure")
		}
	})

	t.Run("PATH warning", func(t *testing.T) {
		archiveBytes, checksum := makeFakeArchive(t)
		archiveName := hostArchiveName(t, "0.1.0")
		checksumContent := fmt.Sprintf("%s  %s\n", checksum, archiveName)

		server := newMockGitHub(t, "0.1.0", archiveBytes, checksumContent)
		defer server.Close()

		prefix := filepath.Join(t.TempDir(), "custom-install")
		result := runScript(t, []string{"--version", "0.1.0", "--prefix", prefix, "--yes"}, []string{
			"MUSH_INSTALL_BASE_URL=" + server.URL,
			"MUSH_INSTALL_INSECURE=1",
		})

		if result.exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s",
				result.exitCode, result.stdout, result.stderr)
		}
		if !strings.Contains(result.stdout, "not in your PATH") {
			t.Errorf("stdout missing PATH warning\ngot: %s", result.stdout)
		}
	})
}
