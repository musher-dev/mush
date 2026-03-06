//go:build unix

package harness

import "github.com/musher-dev/mush/internal/harness/harnesstype"

type (
	Executor          = harnesstype.Executor
	SetupOptions      = harnesstype.SetupOptions
	ExecResult        = harnesstype.ExecResult
	ExecError         = harnesstype.ExecError
	ProviderSpec      = harnesstype.ProviderSpec
	StatusSpec        = harnesstype.StatusSpec
	AuthCheck         = harnesstype.AuthCheck
	MCPSpec           = harnesstype.MCPSpec
	MCPDef            = harnesstype.MCPDef
	MCPProviderSpec   = harnesstype.MCPProviderSpec
	SignalDirConsumer = harnesstype.SignalDirConsumer
)

var (
	BuildMCPProviderSpecs  = harnesstype.BuildMCPProviderSpecs
	BuildJSONMCPConfig     = harnesstype.BuildJSONMCPConfig
	BuildOpenCodeMCPConfig = harnesstype.BuildOpenCodeMCPConfig
	BuildGeminiMCPConfig   = harnesstype.BuildGeminiMCPConfig
	BuildCursorMCPConfig   = harnesstype.BuildCursorMCPConfig
	CreateMCPConfigFile    = harnesstype.CreateMCPConfigFile
	LoadedMCPProviderNames = harnesstype.LoadedMCPProviderNames
)
