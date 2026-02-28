package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/paths"
)

// PathsInfo holds all resolved paths for JSON output.
type PathsInfo struct {
	ConfigRoot  string `json:"config_root"`
	StateRoot   string `json:"state_root"`
	CacheRoot   string `json:"cache_root"`
	ConfigFile  string `json:"config_file"`
	Credentials string `json:"credentials"`
	LogFile     string `json:"log_file"`
	HistoryDir  string `json:"history_dir"`
	BundleCache string `json:"bundle_cache"`
	UpdateState string `json:"update_state"`
	APIURL      string `json:"api_url"`
	AuthSource  string `json:"auth_source"`
}

func newPathsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Show where Mush stores files",
		Long: `Display all file and directory paths used by Mush.

Useful for debugging, scripting, and understanding where configuration,
state, cache, and credential files are stored on this system.`,
		Example: `  mush paths
  mush paths --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			info := resolvePathsInfo()

			if out.JSON {
				return out.PrintJSON(info)
			}

			out.Print("Config root:    %s\n", info.ConfigRoot)
			out.Print("State root:     %s\n", info.StateRoot)
			out.Print("Cache root:     %s\n", info.CacheRoot)
			out.Print("\n")
			out.Print("Config file:    %s\n", info.ConfigFile)
			out.Print("Credentials:    %s\n", info.Credentials)
			out.Print("Log file:       %s\n", info.LogFile)
			out.Print("History dir:    %s\n", info.HistoryDir)
			out.Print("Bundle cache:   %s\n", info.BundleCache)
			out.Print("Update state:   %s\n", info.UpdateState)
			out.Print("\n")
			out.Print("API URL:        %s\n", info.APIURL)
			out.Print("Auth source:    %s\n", info.AuthSource)

			return nil
		},
	}
}

func resolvePathsInfo() PathsInfo {
	info := PathsInfo{}

	info.ConfigRoot = resolveOrError(paths.ConfigRoot)
	info.StateRoot = resolveOrError(paths.StateRoot)
	info.CacheRoot = resolveOrError(paths.CacheRoot)
	info.LogFile = resolveOrError(paths.DefaultLogFile)
	info.HistoryDir = resolveOrError(paths.HistoryDir)
	info.BundleCache = resolveOrError(paths.BundleCacheDir)
	info.UpdateState = resolveOrError(paths.UpdateStateFile)
	info.Credentials = resolveOrError(paths.CredentialsFile)

	if cr := info.ConfigRoot; cr != "" {
		info.ConfigFile = cr + "/config.yaml"
	} else {
		info.ConfigFile = "<error: config root unavailable>"
	}

	cfg := config.Load()
	info.APIURL = cfg.APIURL()

	source, _ := auth.GetCredentials()
	if source == auth.SourceNone {
		info.AuthSource = "none"
	} else {
		info.AuthSource = string(source)
	}

	return info
}

func resolveOrError(fn func() (string, error)) string {
	val, err := fn()
	if err != nil {
		return fmt.Sprintf("<error: %v>", err)
	}

	return val
}
