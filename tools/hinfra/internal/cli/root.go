package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/config"
	mcpsrv "github.com/gabehamasaki/hinfra/tools/hinfra/internal/mcp"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tailnet"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version is set at link time via -ldflags (see .goreleaser.yaml).
var Version = "dev"

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "hinfra",
		Short: "CLI, TUI e MCP para o kit hinfra",
		RunE: func(cmd *cobra.Command, args []string) error {
			if term.IsTerminal(int(os.Stdout.Fd())) {
				return tui.Run(loadEnv())
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "saída JSON")
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newSmokeCmd())
	root.AddCommand(newTUICmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newDeployCmd())
	root.AddCommand(newMetricsCmd())
	root.AddCommand(newHealthCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newDocsCmd())
	root.AddCommand(newRestartCmd())
	root.AddCommand(newArgoCDCmd())
	root.AddCommand(newRollbackCmd())
	root.AddCommand(newScaffoldCmd())
	root.AddCommand(newSealCmd())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(Version)
		},
	})
	return root
}

func loadEnv() *actions.Env {
	runtime, err := config.Load()
	if err != nil {
		fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	return &actions.Env{Runtime: runtime, CWD: cwd}
}

func loadEnvValidated() *actions.Env {
	env := loadEnv()
	if err := env.Runtime.ValidateFiles(); err != nil {
		fatal(err)
	}
	return env
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Valida config, kubeconfig e tailnet",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnv()
			if err := env.Runtime.ValidateFiles(); err != nil {
				fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
		if err := tailnet.Require(ctx, env.Runtime.TailnetAPI, 2*time.Second); err != nil {
			fatal(err)
		}
		if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "infra-mcp", "config.yaml")); err == nil {
			fmt.Println("aviso: migre ~/.config/infra-mcp/config.yaml para ~/.config/hinfra/config.yaml")
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel2()
		if warns, err := actions.GitOpsWarnings(ctx2, env); err == nil {
			if msg := actions.FormatGitOpsWarnings(warns); msg != "" {
				fmt.Println(msg)
			}
		}
		fmt.Println("doctor ok")
		},
	}
}

func newSmokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smoke",
		Short: "Smoke test das ferramentas de leitura no cwd",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if err := mcpsrv.RunSmoke(ctx, env); err != nil {
				fatal(err)
			}
		},
	}
}

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Abre a interface TUI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return tui.Run(loadEnv())
		},
	}
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Servidor MCP stdio",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			srv := mcp.NewServer(&mcp.Implementation{Name: "hinfra", Version: Version}, nil)
			mcpsrv.Register(srv, env)
			if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
				fatal(err)
			}
		},
	}
	cmd.AddCommand(newMCPInstallCmd())
	return cmd
}
