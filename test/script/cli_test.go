package script_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var (
	buildOnce sync.Once
	buildPath string
	errBuild  error
)

func TestCLI(t *testing.T) {
	t.Parallel()

	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		Setup:               setupCLI,
	})
}

func TestDocSnippets(t *testing.T) {
	t.Parallel()

	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/docs",
		RequireExplicitExec: true,
		Setup:               setupCLI,
	})
}

func setupCLI(env *testscript.Env) error {
	homeDir := filepath.Join(env.WorkDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("create script home dir: %w", err)
	}

	xdgConfigDir := filepath.Join(env.WorkDir, "xdg")
	if err := os.MkdirAll(xdgConfigDir, 0o755); err != nil {
		return fmt.Errorf("create script XDG config dir: %w", err)
	}

	mushBin, err := buildCLI()
	if err != nil {
		return err
	}

	env.Setenv("HOME", homeDir)
	env.Setenv("XDG_CONFIG_HOME", xdgConfigDir)
	env.Setenv("MUSH_BIN", mushBin)

	return nil
}

func buildCLI() (string, error) {
	buildOnce.Do(func() {
		exeSuffix := ""
		if runtime.GOOS == "windows" {
			exeSuffix = ".exe"
		}

		repoRoot, err := repoRoot()
		if err != nil {
			errBuild = err
			return
		}

		binDir, err := os.MkdirTemp("", "mush-script-bin-")
		if err != nil {
			errBuild = fmt.Errorf("create shared script bin dir: %w", err)
			return
		}

		buildPath = filepath.Join(binDir, "mush"+exeSuffix)

		buildCmd := exec.CommandContext(context.Background(), "go", "build", "-o", buildPath, "./cmd/mush")
		buildCmd.Dir = repoRoot

		buildCmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mush-gocache"))

		output, err := buildCmd.CombinedOutput()
		if err != nil {
			errBuild = fmt.Errorf("build mush test binary: %w\n%s", err, output)
		}
	})

	if errBuild != nil {
		return "", errBuild
	}

	return buildPath, nil
}

func repoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve test file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..")), nil
}
