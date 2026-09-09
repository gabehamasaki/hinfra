package mcp

import (
	"context"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Env = actions.Env

func Register(server *mcp.Server, env *Env) {
	mcp.AddTool(server, &mcp.Tool{Name: "deploy_status", Description: "Verifica os 4 elos do deploy: SHA em origin/main, bump no infra, Application Synced/Healthy, imagem no pod"}, deployStatusHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_health", Description: "Pods, restarts, imagem em execução e events do namespace"}, appHealthHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_logs", Description: "Logs de pod com suporte a previous=true para CrashLoopBackOff"}, appLogsHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "infra_docs", Description: "Busca e leitura em docs/ do repo infra"}, infraDocsHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "app_restart", Description: "Rollout restart restrito a apps/ do repo infra"}, appRestartHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "argocd_refresh", Description: "Refresh hard no Application correto (root-app para Helm)"}, argocdRefreshHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "rollback", Description: "Rollback GitOps via kustomize edit set image (commit pra frente)"}, rollbackHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "scaffold_app", Description: "Gera manifestos no repo infra (write:false preview por padrão)"}, scaffoldAppHandler(env))
	mcp.AddTool(server, &mcp.Tool{Name: "scaffold_workflow", Description: "Gera workflow de deploy e hinfra.yml no repo do projeto"}, scaffoldWorkflowHandler(env))
}

type deployStatusInput struct {
	App string `json:"app,omitempty"`
}

func deployStatusHandler(env *Env) func(context.Context, *mcp.CallToolRequest, deployStatusInput) (*mcp.CallToolResult, actions.DeployStatusResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deployStatusInput) (*mcp.CallToolResult, actions.DeployStatusResult, error) {
		out, err := actions.DeployStatus(ctx, env, in.App)
		return nil, out, err
	}
}

type appHealthInput struct {
	App string `json:"app,omitempty"`
}

func appHealthHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appHealthInput) (*mcp.CallToolResult, actions.AppHealthResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appHealthInput) (*mcp.CallToolResult, actions.AppHealthResult, error) {
		out, err := actions.AppHealth(ctx, env, in.App)
		return nil, out, err
	}
}

type appLogsInput struct {
	App       string `json:"app,omitempty"`
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container,omitempty"`
	Previous  bool   `json:"previous,omitempty"`
	Tail      int    `json:"tail,omitempty"`
}

func appLogsHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appLogsInput) (*mcp.CallToolResult, actions.AppLogsResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appLogsInput) (*mcp.CallToolResult, actions.AppLogsResult, error) {
		out, err := actions.AppLogs(ctx, env, in.App, actions.AppLogsInput{
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
	App string `json:"app,omitempty"`
}

func appRestartHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appRestartInput) (*mcp.CallToolResult, actions.AppRestartResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appRestartInput) (*mcp.CallToolResult, actions.AppRestartResult, error) {
		out, err := actions.AppRestart(ctx, env, in.App)
		return nil, out, err
	}
}

type argocdRefreshInput struct {
	App string `json:"app,omitempty"`
}

func argocdRefreshHandler(env *Env) func(context.Context, *mcp.CallToolRequest, argocdRefreshInput) (*mcp.CallToolResult, actions.ArgoCDRefreshResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in argocdRefreshInput) (*mcp.CallToolResult, actions.ArgoCDRefreshResult, error) {
		out, err := actions.ArgoCDRefresh(ctx, env, in.App)
		return nil, out, err
	}
}

type rollbackInput struct {
	App     string `json:"app,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

func rollbackHandler(env *Env) func(context.Context, *mcp.CallToolRequest, rollbackInput) (*mcp.CallToolResult, actions.RollbackResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in rollbackInput) (*mcp.CallToolResult, actions.RollbackResult, error) {
		out, err := actions.Rollback(env, in.App, in.Confirm)
		return nil, out, err
	}
}

type scaffoldAppInput struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	ContainerPort int    `json:"containerPort,omitempty"`
	CPURequest    string `json:"cpuRequest,omitempty"`
	MemoryRequest string `json:"memoryRequest,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
	Exposure      string `json:"exposure,omitempty"`
	Write         bool   `json:"write,omitempty"`
}

func scaffoldAppHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldAppInput) (*mcp.CallToolResult, actions.ScaffoldAppResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldAppInput) (*mcp.CallToolResult, actions.ScaffoldAppResult, error) {
		out, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
			Name: in.Name, Host: in.Host, ContainerPort: in.ContainerPort,
			CPURequest: in.CPURequest, MemoryRequest: in.MemoryRequest,
			MemoryLimit: in.MemoryLimit, Exposure: in.Exposure, Write: in.Write,
		})
		return nil, out, err
	}
}

type scaffoldWorkflowInput struct {
	App   string `json:"app,omitempty"`
	Write bool   `json:"write,omitempty"`
}

func scaffoldWorkflowHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldWorkflowInput) (*mcp.CallToolResult, actions.ScaffoldWorkflowResult, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldWorkflowInput) (*mcp.CallToolResult, actions.ScaffoldWorkflowResult, error) {
		out, err := actions.ScaffoldWorkflow(env, in.App, in.Write)
		return nil, out, err
	}
}
