package harnesstype

import (
	"fmt"

	"github.com/musher-dev/mush/internal/executil"
	"gopkg.in/yaml.v3"
)

// ProviderSpec describes a harness provider loaded from an embedded YAML file.
type ProviderSpec struct {
	Name        string         `yaml:"name"`
	DisplayName string         `yaml:"displayName"`
	Description string         `yaml:"description"`
	Binary      string         `yaml:"binary"`
	Directories *Directories   `yaml:"directories,omitempty"`
	BundleDir   *BundleDirSpec `yaml:"bundleDir,omitempty"`
	CLI         *CLIFlags      `yaml:"cli,omitempty"`
	Assets      *AssetPaths    `yaml:"assets,omitempty"`
	MCP         *MCPDef        `yaml:"mcp,omitempty"`
	Status      *StatusSpec    `yaml:"status,omitempty"`
}

// Directories describes harness-specific config directory paths.
type Directories struct {
	Project string `yaml:"project"`
	User    string `yaml:"user"`
}

// BundleDirSpec describes how a bundle directory is injected into the harness CLI.
type BundleDirSpec struct {
	Mode string `yaml:"mode"` // "add_dir", "cd_flag", "cwd"
	Flag string `yaml:"flag"` // CLI flag for add_dir/cd_flag modes
}

// CLIFlags describes harness-specific CLI flags.
type CLIFlags struct {
	MCPConfig string `yaml:"mcpConfig"`
}

// AssetPaths describes where bundle assets are mapped in the harness's native structure.
type AssetPaths struct {
	SkillDir       string `yaml:"skillDir"`
	AgentDir       string `yaml:"agentDir"`
	ToolConfigFile string `yaml:"toolConfigFile"`
}

// MCPDef describes MCP configuration for a harness.
type MCPDef struct {
	Format     string `yaml:"format"`     // "json" or "toml"
	ConfigPath string `yaml:"configPath"` // relative path for MCP config file
}

// StatusSpec describes health-check and install metadata for a harness provider.
type StatusSpec struct {
	VersionArgs    []string   `yaml:"versionArgs,omitempty"`
	InstallHint    string     `yaml:"installHint,omitempty"`
	InstallCommand []string   `yaml:"installCommand,omitempty"`
	ConfigDir      string     `yaml:"configDir,omitempty"`
	AuthCheck      *AuthCheck `yaml:"authCheck,omitempty"`
}

// AuthCheck describes a file-based credential check for a harness provider.
type AuthCheck struct {
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

// MustParseSpec parses a YAML spec from raw bytes and panics on failure.
func MustParseSpec(data []byte) *ProviderSpec {
	var spec ProviderSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		panic(fmt.Sprintf("harnesstype: unmarshal spec: %v", err))
	}

	validateProviderSpec(&spec)

	return &spec
}

func validateProviderSpec(spec *ProviderSpec) {
	if spec.Name == "" {
		panic("harnesstype: spec name is required")
	}

	if spec.BundleDir != nil && spec.BundleDir.Mode != "" {
		switch spec.BundleDir.Mode {
		case "add_dir", "cd_flag", "cwd":
			// valid
		default:
			panic(fmt.Sprintf("harnesstype: provider %s: invalid bundleDir.mode %q", spec.Name, spec.BundleDir.Mode))
		}
	}

	if spec.MCP != nil && spec.MCP.Format != "" {
		switch spec.MCP.Format {
		case "json", "toml":
			// valid
		default:
			panic(fmt.Sprintf("harnesstype: provider %s: invalid mcp.format %q", spec.Name, spec.MCP.Format))
		}
	}
}

// AvailableFunc returns a lazy closure that checks if a provider's binary is available.
func AvailableFunc(spec *ProviderSpec) func() bool {
	return func() bool {
		if spec.Binary == "" {
			return true
		}

		_, err := executil.LookPath(spec.Binary)

		return err == nil
	}
}
