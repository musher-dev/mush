package harness

import (
	"embed"
	"fmt"
	"os/exec"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed providers/*.yaml
var providersFS embed.FS

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

// providerSpecs is loaded at package init time from embedded YAML files.
var providerSpecs = mustLoadProviders(providersFS)

func mustLoadProviders(fsys embed.FS) map[string]*ProviderSpec {
	entries, err := fsys.ReadDir("providers")
	if err != nil {
		panic(fmt.Sprintf("harness: read providers dir: %v", err))
	}

	specs := make(map[string]*ProviderSpec, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, readErr := fsys.ReadFile("providers/" + entry.Name())
		if readErr != nil {
			panic(fmt.Sprintf("harness: read provider file %s: %v", entry.Name(), readErr))
		}

		var spec ProviderSpec
		if unmarshalErr := yaml.Unmarshal(data, &spec); unmarshalErr != nil {
			panic(fmt.Sprintf("harness: unmarshal provider %s: %v", entry.Name(), unmarshalErr))
		}

		validateProviderSpec(&spec, entry.Name())

		if _, dup := specs[spec.Name]; dup {
			panic(fmt.Sprintf("harness: duplicate provider name %q in %s", spec.Name, entry.Name()))
		}

		specs[spec.Name] = &spec
	}

	return specs
}

func validateProviderSpec(spec *ProviderSpec, filename string) {
	if spec.Name == "" {
		panic(fmt.Sprintf("harness: provider %s: name is required", filename))
	}

	if spec.BundleDir != nil && spec.BundleDir.Mode != "" {
		switch spec.BundleDir.Mode {
		case "add_dir", "cd_flag", "cwd":
			// valid
		default:
			panic(fmt.Sprintf("harness: provider %s: invalid bundleDir.mode %q", filename, spec.BundleDir.Mode))
		}
	}

	if spec.MCP != nil && spec.MCP.Format != "" {
		switch spec.MCP.Format {
		case "json", "toml":
			// valid
		default:
			panic(fmt.Sprintf("harness: provider %s: invalid mcp.format %q", filename, spec.MCP.Format))
		}
	}
}

// GetProvider returns the ProviderSpec for a named harness type.
func GetProvider(name string) (*ProviderSpec, bool) {
	spec, ok := providerSpecs[name]
	return spec, ok
}

// ProviderNames returns all provider names in sorted order.
func ProviderNames() []string {
	names := make([]string, 0, len(providerSpecs))
	for name := range providerSpecs {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// HasAssetMapping returns true if the named provider has asset mapping rules.
func HasAssetMapping(name string) bool {
	spec, ok := providerSpecs[name]
	return ok && spec.Assets != nil
}

// AvailableFunc returns a lazy closure that checks if a provider's binary is available.
// The closure reads from the provider spec map at call time, avoiding init-order dependence.
func AvailableFunc(name string) func() bool {
	return func() bool {
		spec, ok := providerSpecs[name]
		if !ok {
			return false
		}

		if spec.Binary == "" {
			return true
		}

		_, err := exec.LookPath(spec.Binary)

		return err == nil
	}
}
