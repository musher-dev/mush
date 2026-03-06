//go:build unix

package opencode

import "github.com/musher-dev/mush/internal/harness/harnesstype"

type (
	SetupOptions = harnesstype.SetupOptions
	ExecResult   = harnesstype.ExecResult
	ExecError    = harnesstype.ExecError
)
