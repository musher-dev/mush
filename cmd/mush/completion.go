package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Example: `  mush completion bash
  source <(mush completion bash)`,
		Long: `Generate shell completion scripts for Mush CLI.

To load completions:

Bash:
  $ source <(mush completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ mush completion bash > /etc/bash_completion.d/mush
  # macOS:
  $ mush completion bash > $(brew --prefix)/etc/bash_completion.d/mush

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ mush completion zsh > "${fpath[1]}/_mush"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ mush completion fish | source

  # To load completions for each session, execute once:
  $ mush completion fish > ~/.config/fish/completions/mush.fish

PowerShell:
  PS> mush completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> mush completion powershell > mush.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}

			return nil
		},
	}

	return cmd
}
