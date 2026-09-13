package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/initwizard"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/mcpinstall"
	"github.com/spf13/cobra"
)

func ctx60() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

func newDeployCmd() *cobra.Command {
	var app string
	var deployEnv string
	cmd := &cobra.Command{Use: "deploy", Short: "Comandos de deploy"}
	status := &cobra.Command{
		Use:   "status",
		Short: "Verifica os 4 elos do deploy",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			out, err := actions.DeployStatusForEnv(ctx, env, app, deployEnv)
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				fmt.Printf("step %d ok=%v version=%s\n%s\n", out.Step, out.OK, out.Version, out.Detail)
			}); err != nil {
				fatal(err)
			}
			if !out.OK {
				osExit(1)
			}
		},
	}
	status.Flags().StringVar(&app, "app", "", "override do app")
	status.Flags().StringVar(&deployEnv, "env", "production", "ambiente: production, dev ou homolog")
	cmd.AddCommand(status)
	return cmd
}

func newHealthCmd() *cobra.Command {
	var app string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Pods e events do namespace",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			out, err := actions.AppHealth(ctx, env, app)
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				for _, p := range out.Pods {
					fmt.Printf("%s %s restarts=%d %s\n", p.Name, p.Phase, p.Restarts, p.Image)
				}
				for _, e := range out.Events {
					fmt.Println(e)
				}
			}); err != nil {
				fatal(err)
			}
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "override do app")
	return cmd
}

func newLogsCmd() *cobra.Command {
	var app, pod, container string
	var previous bool
	var tail int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Logs de pod",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			out, err := actions.AppLogs(ctx, env, app, actions.AppLogsInput{
				Pod: pod, Container: container, Previous: previous, Tail: tail,
			})
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				fmt.Printf("# pod: %s\n%s", out.Pod, out.Logs)
			}); err != nil {
				fatal(err)
			}
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "override do app")
	cmd.Flags().StringVar(&pod, "pod", "", "nome do pod")
	cmd.Flags().StringVar(&container, "container", "", "container")
	cmd.Flags().BoolVar(&previous, "previous", false, "logs do container anterior")
	cmd.Flags().IntVar(&tail, "tail", 100, "linhas")
	return cmd
}

func newDocsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "docs", Short: "Documentação do repo infra"}
	search := &cobra.Command{
		Use:   "search [query]",
		Short: "Busca em docs/",
		Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			env := loadEnv()
			out, err := actions.InfraDocs(env, args[0], "")
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				for _, r := range out.Results {
					fmt.Printf("%s:%d %s\n", r.File, r.Line, r.Snippet)
				}
			}); err != nil {
				fatal(err)
			}
		},
	}
	read := &cobra.Command{
		Use:   "read [path]",
		Short: "Lê arquivo em docs/",
		Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			env := loadEnv()
			out, err := actions.InfraDocs(env, "", args[0])
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() { fmt.Print(out.Content) }); err != nil {
				fatal(err)
			}
		},
	}
	cmd.AddCommand(search, read)
	return cmd
}

func newRestartCmd() *cobra.Command {
	var app string
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Rollout restart",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			out, err := actions.AppRestart(ctx, env, app)
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() { fmt.Println(out.Message) }); err != nil {
				fatal(err)
			}
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "override do app")
	return cmd
}

func newArgoCDCmd() *cobra.Command {
	var app string
	var root bool
	cmd := &cobra.Command{Use: "argocd", Short: "Comandos ArgoCD"}
	refresh := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh hard no Application",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			var out actions.ArgoCDRefreshResult
			var err error
			if root {
				out, err = actions.ArgoCDRefreshRoot(ctx, env)
			} else {
				out, err = actions.ArgoCDRefresh(ctx, env, app)
			}
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				fmt.Printf("%s (target=%s)\n", out.Message, out.Target)
			}); err != nil {
				fatal(err)
			}
		},
	}
	refresh.Flags().StringVar(&app, "app", "", "override do app")
	refresh.Flags().BoolVar(&root, "root", false, "refresh hard no Application root (argocdRootApp na config; default workloads-root se workloadsRepo separado)")
	cmd.AddCommand(refresh)

	var force, projects, wait, noRefresh bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh + sync (aplica Git no cluster; use após mudar manifestos)",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			out, err := actions.ArgoCDSync(ctx, env, actions.ArgoCDSyncInput{
				AppOverride: app,
				Root:        root,
				Projects:    projects,
				Force:       force,
				Refresh:     !noRefresh,
				Wait:        wait,
			})
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				fmt.Println(out.Message)
			}); err != nil {
				fatal(err)
			}
		},
	}
	syncCmd.Flags().StringVar(&app, "app", "", "override do app (default: contexto do cwd / hinfra.yml)")
	syncCmd.Flags().BoolVar(&root, "root", false, "sync no Application root (argocdRootApp na config)")
	syncCmd.Flags().BoolVar(&projects, "projects", false, "sync todos os Applications em apps/ (my-portfolio, study, …)")
	syncCmd.Flags().BoolVar(&force, "force", true, "sync com Force=true (recursos que resistem ao apply)")
	syncCmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "não fazer refresh hard antes do sync")
	syncCmd.Flags().BoolVar(&wait, "wait", true, "aguardar operação terminar (até 3 min por app)")
	cmd.AddCommand(syncCmd)
	return cmd
}

func newSealCmd() *cobra.Command {
	var app, ns, input, output string
	var execute bool
	cmd := &cobra.Command{
		Use:   "seal",
		Short: "Sealed Secrets via kubeseal",
	}
	secret := &cobra.Command{
		Use:   "secret",
		Short: "Criptografa Secret em sealed-secret.yaml",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			out, err := actions.SealSecret(env, actions.SealSecretInput{
				AppOverride: app, Namespace: ns, InputFile: input, OutputFile: output, Execute: execute,
			})
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				if out.Command != "" {
					fmt.Println(out.Command)
				}
				if out.Message != "" {
					fmt.Println(out.Message)
				}
				if out.WroteTo != "" {
					fmt.Println(out.WroteTo)
				}
			}); err != nil {
				fatal(err)
			}
		},
	}
	secret.Flags().StringVar(&app, "app", "", "override do app")
	secret.Flags().StringVar(&ns, "namespace", "", "namespace do Secret")
	secret.Flags().StringVarP(&input, "file", "f", "", "Secret kubernetes em yaml (plaintext local)")
	secret.Flags().StringVarP(&output, "output", "o", "", "caminho de saída no repo infra")
	secret.Flags().BoolVar(&execute, "execute", false, "executar kubeseal (senão só preview)")
	cmd.AddCommand(secret)
	return cmd
}

func newRollbackCmd() *cobra.Command {
	var app string
	var confirm bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback GitOps via kustomize",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			out, err := actions.Rollback(env, app, confirm)
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() {
				if out.Message != "" {
					fmt.Println(out.Message)
				} else {
					fmt.Println(out.Preview)
				}
			}); err != nil {
				fatal(err)
			}
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "override do app")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "executar rollback")
	return cmd
}

func newScaffoldCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "scaffold", Short: "Gera manifestos e workflows"}
	var write bool
	appCmd := &cobra.Command{Use: "app", Short: "Manifestos no repo infra"}
	var name, host, exposure, cpu, memReq, memLim string
	var port int
	var writeApp bool
	var monorepoApp bool
	appCmd.Flags().StringVar(&name, "name", "", "nome do app")
	appCmd.Flags().StringVar(&host, "host", "", "host")
	appCmd.Flags().IntVar(&port, "container-port", 8080, "porta")
	appCmd.Flags().StringVar(&cpu, "cpu-request", "", "cpu request")
	appCmd.Flags().StringVar(&memReq, "memory-request", "", "memory request")
	appCmd.Flags().StringVar(&memLim, "memory-limit", "", "memory limit")
	appCmd.Flags().StringVar(&exposure, "exposure", "public", "public ou tailnet")
	appCmd.Flags().BoolVar(&writeApp, "write", false, "gravar arquivos")
	appCmd.Flags().BoolVar(&monorepoApp, "monorepo", false, "api+web (schedule-visits pattern)")
	appCmd.Run = func(_ *cobra.Command, _ []string) {
		env := loadEnv()
		out, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
			Name: name, Host: host, ContainerPort: port,
			CPURequest: cpu, MemoryRequest: memReq, MemoryLimit: memLim,
			Exposure: exposure, Write: writeApp, Monorepo: monorepoApp,
		})
		if err != nil {
			fatal(err)
		}
		if err := printOrJSON(out, func() {
			fmt.Println(out.Content)
			if out.DNS != "" {
				fmt.Println(out.DNS)
			}
			for _, c := range out.Checklist {
				fmt.Println("-", c)
			}
		}); err != nil {
			fatal(err)
		}
	}
	wf := &cobra.Command{Use: "workflow", Short: "Workflow e hinfra.yml no projeto"}
	var app string
	var monorepo bool
	wf.Flags().StringVar(&app, "app", "", "override do app")
	wf.Flags().BoolVar(&monorepo, "monorepo", false, "gerar template api+web mesmo sem hinfra.yml")
	wf.Flags().BoolVar(&write, "write", false, "gravar arquivos")
	wf.Run = func(_ *cobra.Command, _ []string) {
		env := loadEnvValidated()
		out, err := actions.ScaffoldWorkflow(env, actions.ScaffoldWorkflowInput{
			AppOverride: app, Write: write, Monorepo: monorepo,
		})
		if err != nil {
			fatal(err)
		}
		if err := printOrJSON(out, func() {
			if out.Message != "" {
				fmt.Println(out.Message)
			} else {
				fmt.Println("--- hinfra.yml ---")
				fmt.Println(out.Hinfra)
				fmt.Println("--- workflow ---")
				fmt.Println(out.Workflow)
			}
			for _, c := range out.Checklist {
				fmt.Println("-", c)
			}
		}); err != nil {
			fatal(err)
		}
	}
	cmd.AddCommand(appCmd, wf)
	return cmd
}

func newInitCmd() *cobra.Command {
	var machine bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Setup de máquina ou onboarding de projeto",
		RunE: func(_ *cobra.Command, _ []string) error {
			if machine {
				return initwizard.RunMachine()
			}
			return initwizard.RunProject(loadEnv())
		},
	}
	cmd.Flags().BoolVar(&machine, "machine", false, "setup da máquina (~/.config/hinfra)")
	return cmd
}

func newMCPInstallCmd() *cobra.Command {
	var cursor, claude, codex, opencode, all, list bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Registra hinfra mcp nos coding agents",
		RunE: func(_ *cobra.Command, _ []string) error {
			if list {
				mcpinstall.ListAgents()
				return nil
			}
			selected := mcpinstall.Selection{Cursor: cursor, Claude: claude, Codex: codex, OpenCode: opencode}
			if all {
				selected = mcpinstall.Selection{Cursor: true, Claude: true, Codex: true, OpenCode: true}
			}
			if !selected.Any() {
				return mcpinstall.RunWizard()
			}
			return mcpinstall.Install(selected)
		},
	}
	cmd.Flags().BoolVar(&cursor, "cursor", false, "configurar Cursor")
	cmd.Flags().BoolVar(&claude, "claude", false, "configurar Claude Code")
	cmd.Flags().BoolVar(&codex, "codex", false, "configurar Codex")
	cmd.Flags().BoolVar(&opencode, "opencode", false, "configurar OpenCode")
	cmd.Flags().BoolVar(&all, "all", false, "todos os agents")
	cmd.Flags().BoolVar(&list, "list", false, "listar agents suportados")
	return cmd
}

func osExit(code int) {
	os.Exit(code)
}
