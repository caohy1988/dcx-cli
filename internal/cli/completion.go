package cli

import (
	"os"

	"github.com/haiyuan-eng-google/dcx-cli/internal/contracts"
	"github.com/spf13/cobra"
)

func (a *App) addCompletionCommand() {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate a shell completion script for dcx.

To load completions:

  bash:
    source <(dcx completion bash)
    # Or add to ~/.bashrc:
    echo 'source <(dcx completion bash)' >> ~/.bashrc

  zsh:
    source <(dcx completion zsh)
    # Or install permanently:
    dcx completion zsh > "${fpath[1]}/_dcx"

  fish:
    dcx completion fish | source
    # Or install permanently:
    dcx completion fish > ~/.config/fish/completions/dcx.fish

  powershell:
    dcx completion powershell | Out-String | Invoke-Expression
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return a.Root.GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return a.Root.GenZshCompletion(os.Stdout)
			case "fish":
				return a.Root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return a.Root.GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}

	a.Root.AddCommand(cmd)

	a.Registry.Register(contracts.BuildContract(
		"completion", "cli",
		"Generate shell completion script",
		[]contracts.FlagContract{
			{Name: "shell", Type: "string", Description: "Shell type: bash, zsh, fish, or powershell", Required: true},
		},
		false, false,
	))
}
