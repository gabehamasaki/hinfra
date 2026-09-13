package mcp

import (
	"context"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Env = actions.Env

func Register(server *mcp.Server, env *Env) {
	mcp.AddTool(server, &mcp.Tool{Name: "deploy_status", Description: "Verifica os 4 elos do deploy: versão no kustomization, bump no infra, Application Synced/Healthy, imagem no pod"}, deployStatusHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_health", Description: "Pods, restarts, imagem em execução e events do namespace"}, appHealthHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_logs", Description: "Logs de pod com suporte a previous=true para CrashLoopBackOff"}, appLogsHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "infra_docs", Description: "Busca e leitura em docs/ do repo infra"}, infraDocsHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_restart", Description: "Rollout restart restrito a apps/ do repo infra"}, appRestartHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "argocd_refresh", Description: "Refresh hard (use root:true após novo Application no infra)"}, argocdRefreshHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "rollback", Description: "Rollback GitOps via kustomize edit set image (commit pra frente)"}, rollbackHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "scaffold_app", Description: "Gera manifestos no repo infra (write:false preview por padrão)"}, scaffoldAppHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "scaffold_workflow", Description: "Gera workflow de deploy e hinfra.yml no repo do projeto"}, scaffoldWorkflowHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "seal_secret", Description: "Gera sealed-secret.yaml via kubeseal (preview ou execução)"}, sealSecretHandler(env))
}

type deployStatusInput struct {
	App        string `json:"app,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
	Env        string `json:"env,omitempty"`
}

func deployStatusHandler(env *Env) func(context.Context, *mcp.CallToolRequest, deployStatusInput) (*mcp.CallToolResult, actions.DeployStatusResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deployStatusInput) (*mcp.CallToolResult, actions.DeployStatusResult, error) {
		deployEnv := in.Env
		if deployEnv == "" {
			deployEnv = "production"
		}
		out, err := actions.DeployStatusForEnv(ctx, withProjectDir(env, in.ProjectDir), in.App, deployEnv)
		return nil, out, err
	}
}

type appHealthInput struct {
	App        string `json:"app,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func appHealthHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appHealthInput) (*mcp.CallToolResult, actions.AppHealthResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appHealthInput) (*mcp.CallToolResult, actions.AppHealthResult, error) {
		out, err := actions.AppHealth(ctx, withProjectDir(env, in.ProjectDir), in.App)
		return nil, out, err
	}
}

type appLogsInput struct {
	App        string `json:"app,omitempty"`
	Pod        string `json:"pod,omitempty"`
	Container  string `json:"container,omitempty"`
	Previous   bool   `json:"previous,omitempty"`
	Tail       int    `json:"tail,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func appLogsHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appLogsInput) (*mcp.CallToolResult, actions.AppLogsResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appLogsInput) (*mcp.CallToolResult, actions.AppLogsResult, error) {
		out, err := actions.AppLogs(ctx, withProjectDir(env, in.ProjectDir), in.App, actions.AppLogsInput{
			Pod: in.Pod, Container: in.Container, Previous: in.Previous, Tail: in.Tail,
		})
		return nil, out, err
	}
}

type infraDocsInput struct {
	Search string `json:"search,omitempty"`
	Read   string `json:"read,omitempty"`
}

func infraDocsHandler(env *Env) func(context.Context, *mcp.CallToolRequest, infraDocsInput) (*mcp.CallToolResult, actions.InfraDocsResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in infraDocsInput) (*mcp.CallToolResult, actions.InfraDocsResult, error) {
		out, err := actions.InfraDocs(env, in.Search, in.Read)
		return nil, out, err
	}
}

type appRestartInput struct {
	App        string `json:"app,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func appRestartHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appRestartInput) (*mcp.CallToolResult, actions.AppRestartResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appRestartInput) (*mcp.CallToolResult, actions.AppRestartResult, error) {
		out, err := actions.AppRestart(ctx, withProjectDir(env, in.ProjectDir), in.App)
		return nil, out, err
	}
}

type argocdRefreshInput struct {
	App        string `json:"app,omitempty"`
	Root       bool   `json:"root,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func argocdRefreshHandler(env *Env) func(context.Context, *mcp.CallToolRequest, argocdRefreshInput) (*mcp.CallToolResult, actions.ArgoCDRefreshResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in argocdRefreshInput) (*mcp.CallToolResult, actions.ArgoCDRefreshResult, error) {
		e := withProjectDir(env, in.ProjectDir)
		if in.Root {
			out, err := actions.ArgoCDRefreshRoot(ctx, e)
			return nil, out, err
		}
		out, err := actions.ArgoCDRefresh(ctx, e, in.App)
		return nil, out, err
	}
}

type rollbackInput struct {
	App        string `json:"app,omitempty"`
	Confirm    bool   `json:"confirm,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func rollbackHandler(env *Env) func(context.Context, *mcp.CallToolRequest, rollbackInput) (*mcp.CallToolResult, actions.RollbackResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in rollbackInput) (*mcp.CallToolResult, actions.RollbackResult, error) {
		out, err := actions.Rollback(withProjectDir(env, in.ProjectDir), in.App, in.Confirm)
		return nil, out, err
	}
}

type scaffoldAppInput struct {
	Name            string `json:"name"`
	Host            string `json:"host"`
	ContainerPort   int    `json:"containerPort,omitempty"`
	CPURequest      string `json:"cpuRequest,omitempty"`
	MemoryRequest   string `json:"memoryRequest,omitempty"`
	MemoryLimit     string `json:"memoryLimit,omitempty"`
	Exposure        string `json:"exposure,omitempty"`
	Write           bool   `json:"write,omitempty"`
	Monorepo        bool   `json:"monorepo,omitempty"`
	ApiPort         int    `json:"apiPort,omitempty"`
	WebPort         int    `json:"webPort,omitempty"`
	NeedsMigration  bool   `json:"needsMigration,omitempty"`
	NeedsSealedSecret bool `json:"needsSealedSecret,omitempty"`
	MigrationCommand string `json:"migrationCommand,omitempty"`
}

func scaffoldAppHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldAppInput) (*mcp.CallToolResult, actions.ScaffoldAppResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldAppInput) (*mcp.CallToolResult, actions.ScaffoldAppResult, error) {
		out, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
			Name: in.Name, Host: in.Host, ContainerPort: in.ContainerPort,
			CPURequest: in.CPURequest, MemoryRequest: in.MemoryRequest,
			MemoryLimit: in.MemoryLimit, Exposure: in.Exposure, Write: in.Write,
			Monorepo: in.Monorepo, ApiPort: in.ApiPort, WebPort: in.WebPort,
			NeedsMigration: in.NeedsMigration, NeedsSealedSecret: in.NeedsSealedSecret,
			MigrationCommand: in.MigrationCommand,
		})
		return nil, out, err
	}
}

type scaffoldWorkflowInput struct {
	App        string `json:"app,omitempty"`
	Write      bool   `json:"write,omitempty"`
	Monorepo   bool   `json:"monorepo,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func scaffoldWorkflowHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldWorkflowInput) (*mcp.CallToolResult, actions.ScaffoldWorkflowResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldWorkflowInput) (*mcp.CallToolResult, actions.ScaffoldWorkflowResult, error) {
		out, err := actions.ScaffoldWorkflow(withProjectDir(env, in.ProjectDir), actions.ScaffoldWorkflowInput{
			AppOverride: in.App, Write: in.Write, Monorepo: in.Monorepo,
		})
		return nil, out, err
	}
}

type sealSecretInput struct {
	App        string `json:"app,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	SecretName string `json:"secretName,omitempty"`
	InputFile  string `json:"inputFile,omitempty"`
	OutputFile string `json:"outputFile,omitempty"`
	Execute    bool   `json:"execute,omitempty"`
	ProjectDir string `json:"projectDir,omitempty"`
}

func sealSecretHandler(env *Env) func(context.Context, *mcp.CallToolRequest, sealSecretInput) (*mcp.CallToolResult, actions.SealSecretResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in sealSecretInput) (*mcp.CallToolResult, actions.SealSecretResult, error) {
		out, err := actions.SealSecret(withProjectDir(env, in.ProjectDir), actions.SealSecretInput{
			AppOverride: in.App, Namespace: in.Namespace, SecretName: in.SecretName,
			InputFile: in.InputFile, OutputFile: in.OutputFile, Execute: in.Execute,
		})
		return nil, out, err
	}
}
