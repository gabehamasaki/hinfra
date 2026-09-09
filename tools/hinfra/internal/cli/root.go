package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	mcpsrv "github.com/gabehamasaki/infra/tools/hinfra/internal/mcp"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tailnet"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const version = "0.1.0"

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "hinfra",
		Short: "CLI, TUI e MCP para infra hamasakis",
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
			srv := mcp.NewServer(&mcp.Implementation{Name: "infra", Version: version}, nil)
			mcpsrv.Register(srv, env)
			if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
				fatal(err)
			}
		},
	}
	cmd.AddCommand(newMCPInstallCmd())
	return cmd
}
