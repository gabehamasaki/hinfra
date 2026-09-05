package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gabehamasaki/infra/tools/mcp/internal/argocd"
	"github.com/gabehamasaki/infra/tools/mcp/internal/config"
	appctx "github.com/gabehamasaki/infra/tools/mcp/internal/context"
	docstore "github.com/gabehamasaki/infra/tools/mcp/internal/docs"
	"github.com/gabehamasaki/infra/tools/mcp/internal/git"
	"github.com/gabehamasaki/infra/tools/mcp/internal/k8s"
	"github.com/gabehamasaki/infra/tools/mcp/internal/scaffold"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Env struct {
	Runtime *config.Runtime
	CWD     string
}

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

func resolveApp(env *Env, appOverride string) (*appctx.AppContext, error) {
	resolver := appctx.NewResolver(env.Runtime.InfraRepo, env.Runtime.Kubeconfig)
	return resolver.Resolve(env.CWD, appOverride)
}

type deployStatusInput struct {
	App string `json:"app,omitempty"`
}

type deployStatusOutput struct {
	Step   int    `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	SHA    string `json:"sha,omitempty"`
}

func deployStatusHandler(env *Env) func(context.Context, *mcp.CallToolRequest, deployStatusInput) (*mcp.CallToolResult, deployStatusOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deployStatusInput) (*mcp.CallToolResult, deployStatusOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, deployStatusOutput{Step: 0, OK: false, Detail: err.Error()}, nil
		}
		runner := git.NewRunner(app.ProjectRepo)
		sha, err := runner.LSRemote("refs/heads/main")
		if err != nil {
			return nil, deployStatusOutput{Step: 1, OK: false, Detail: err.Error()}, nil
		}
		infraGit := git.NewRunner(app.InfraRepo)
		logOut, err := infraGit.LogGrep(sha, 5)
		if err != nil || logOut == "" {
			detail := "commit de bump não encontrado no repo infra para SHA " + sha
			if err != nil {
				detail = err.Error()
			}
			return nil, deployStatusOutput{Step: 2, OK: false, Detail: detail, SHA: sha}, nil
		}
		kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
		if err != nil {
			return nil, deployStatusOutput{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
		}
		ac, err := argocd.NewClient(kc.Config)
		if err != nil {
			return nil, deployStatusOutput{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
		}
		st, err := ac.GetApplication(ctx, app.Name)
		if err != nil {
			return nil, deployStatusOutput{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
		}
		if st.Sync != "Synced" || st.Health != "Healthy" {
			return nil, deployStatusOutput{Step: 3, OK: false, Detail: argocd.FormatAppStatus(st), SHA: sha}, nil
		}
		pods, err := k8s.ListPods(ctx, kc, app.Namespace)
		if err != nil {
			return nil, deployStatusOutput{Step: 4, OK: false, Detail: err.Error(), SHA: sha}, nil
		}
		podImages := make([]string, 0, len(pods))
		for _, p := range pods {
			podImages = append(podImages, p.Image)
		}
		if !appctx.ImagesDeployed(podImages, app.AllImageRefs(), sha) {
			return nil, deployStatusOutput{
				Step: 4, OK: false, SHA: sha,
				Detail: fmt.Sprintf("pod(s) não rodam todas as imagens com SHA %s; imagens atuais: %v; esperadas: %v", sha, podImages, app.AllImageRefs()),
			}, nil
		}
		return nil, deployStatusOutput{Step: 4, OK: true, Detail: "deploy completo", SHA: sha}, nil
	}
}

type appHealthInput struct {
	App string `json:"app,omitempty"`
}

type appHealthOutput struct {
	Pods   []k8s.PodSummary `json:"pods"`
	Events []string         `json:"events"`
}

func appHealthHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appHealthInput) (*mcp.CallToolResult, appHealthOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appHealthInput) (*mcp.CallToolResult, appHealthOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, appHealthOutput{}, err
		}
		kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
		if err != nil {
			return nil, appHealthOutput{}, err
		}
		pods, err := k8s.ListPods(ctx, kc, app.Namespace)
		if err != nil {
			return nil, appHealthOutput{}, err
		}
		events, err := k8s.ListEvents(ctx, kc, app.Namespace)
		if err != nil {
			return nil, appHealthOutput{}, err
		}
		ev := make([]string, 0, len(events))
		for _, e := range events {
			ev = append(ev, fmt.Sprintf("%s %s: %s", e.Reason, e.InvolvedObject.Name, e.Message))
		}
		return nil, appHealthOutput{Pods: pods, Events: ev}, nil
	}
}

type appLogsInput struct {
	App       string `json:"app,omitempty"`
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container,omitempty"`
	Previous  bool   `json:"previous,omitempty"`
	Tail      int    `json:"tail,omitempty"`
}

type appLogsOutput struct {
	Logs string `json:"logs"`
	Pod  string `json:"pod"`
}

func appLogsHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appLogsInput) (*mcp.CallToolResult, appLogsOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appLogsInput) (*mcp.CallToolResult, appLogsOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, appLogsOutput{}, err
		}
		kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
		if err != nil {
			return nil, appLogsOutput{}, err
		}
		pod := in.Pod
		if pod == "" {
			pods, err := k8s.ListPods(ctx, kc, app.Namespace)
			if err != nil {
				return nil, appLogsOutput{}, err
			}
			if len(pods) == 0 {
				return nil, appLogsOutput{}, fmt.Errorf("nenhum pod em namespace %s", app.Namespace)
			}
			pod = pods[0].Name
		}
		tail := int64(100)
		if in.Tail > 0 {
			tail = int64(in.Tail)
		}
		logs, err := k8s.GetPodLogs(ctx, kc, app.Namespace, pod, in.Container, in.Previous, tail)
		if err != nil {
			return nil, appLogsOutput{}, err
		}
		return nil, appLogsOutput{Logs: logs, Pod: pod}, nil
	}
}

type infraDocsInput struct {
	Search string `json:"search,omitempty"`
	Read   string `json:"read,omitempty"`
}

type infraDocsOutput struct {
	Content string            `json:"content,omitempty"`
	Results []docstore.SearchResult `json:"results,omitempty"`
}

func infraDocsHandler(env *Env) func(context.Context, *mcp.CallToolRequest, infraDocsInput) (*mcp.CallToolResult, infraDocsOutput, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in infraDocsInput) (*mcp.CallToolResult, infraDocsOutput, error) {
		store := docstore.NewStore(env.Runtime.InfraRepo)
		if in.Read != "" {
			content, err := store.Read(in.Read)
			if err != nil {
				return nil, infraDocsOutput{}, err
			}
			return nil, infraDocsOutput{Content: content}, nil
		}
		if in.Search != "" {
			results, err := store.Search(in.Search)
			if err != nil {
				return nil, infraDocsOutput{}, err
			}
			return nil, infraDocsOutput{Results: results}, nil
		}
		return nil, infraDocsOutput{}, fmt.Errorf("informe search ou read")
	}
}

type appRestartInput struct {
	App string `json:"app,omitempty"`
}

type appRestartOutput struct {
	Message string `json:"message"`
}

func appRestartHandler(env *Env) func(context.Context, *mcp.CallToolRequest, appRestartInput) (*mcp.CallToolResult, appRestartOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in appRestartInput) (*mcp.CallToolResult, appRestartOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, appRestartOutput{}, err
		}
		if !appctx.AppDirExists(app.InfraRepo, app.Namespace) {
			return nil, appRestartOutput{}, fmt.Errorf("namespace %s não corresponde a apps/ no repo infra", app.Namespace)
		}
		kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
		if err != nil {
			return nil, appRestartOutput{}, err
		}
		if err := k8s.RestartDeployment(ctx, kc, app.Namespace, app.Name); err != nil {
			return nil, appRestartOutput{}, err
		}
		return nil, appRestartOutput{Message: fmt.Sprintf("rollout restart em deployment/%s namespace %s", app.Name, app.Namespace)}, nil
	}
}

type argocdRefreshInput struct {
	App string `json:"app,omitempty"`
}

type argocdRefreshOutput struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

func argocdRefreshHandler(env *Env) func(context.Context, *mcp.CallToolRequest, argocdRefreshInput) (*mcp.CallToolResult, argocdRefreshOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in argocdRefreshInput) (*mcp.CallToolResult, argocdRefreshOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, argocdRefreshOutput{}, err
		}
		kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
		if err != nil {
			return nil, argocdRefreshOutput{}, err
		}
		ac, err := argocd.NewClient(kc.Config)
		if err != nil {
			return nil, argocdRefreshOutput{}, err
		}
		path, hasChart, err := ac.SourcePath(ctx, app.Name)
		if err != nil {
			return nil, argocdRefreshOutput{}, err
		}
		target := argocd.RefreshTarget(app.Name, path, hasChart)
		if err := ac.Refresh(ctx, target); err != nil {
			return nil, argocdRefreshOutput{}, err
		}
		return nil, argocdRefreshOutput{Target: target, Message: "refresh hard aplicado"}, nil
	}
}

type rollbackInput struct {
	App     string `json:"app,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

type rollbackOutput struct {
	Preview string `json:"preview,omitempty"`
	Message string `json:"message,omitempty"`
}

func rollbackHandler(env *Env) func(context.Context, *mcp.CallToolRequest, rollbackInput) (*mcp.CallToolResult, rollbackOutput, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in rollbackInput) (*mcp.CallToolResult, rollbackOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, rollbackOutput{}, err
		}
		runner := git.NewRunner(app.InfraRepo)
		if err := runner.PullRebase(); err != nil {
			return nil, rollbackOutput{}, err
		}
		kustPath := filepath.Join(app.InfraRepo, app.AppPath, "kustomization.yaml")
		_, currentTag, err := appctx.ImageFromKustomization(kustPath)
		if err != nil {
			return nil, rollbackOutput{}, err
		}
		prevTag, err := previousImageTag(runner, app.AppPath+"/kustomization.yaml", currentTag)
		if err != nil {
			return nil, rollbackOutput{}, err
		}
		preview := fmt.Sprintf("rollback de %s para %s em %s", currentTag, prevTag, kustPath)
		if !in.Confirm {
			return nil, rollbackOutput{Preview: preview + " — passe confirm:true para executar"}, nil
		}
		appDir := filepath.Join(app.InfraRepo, app.AppPath)
		for _, ref := range app.AllImageRefs() {
			cmd := exec.Command("kustomize", "edit", "set", "image", ref+"="+ref+":"+prevTag)
			cmd.Dir = appDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return nil, rollbackOutput{}, fmt.Errorf("kustomize (%s): %s: %w", ref, out, err)
			}
		}
		if err := runner.Add(app.AppPath+"/kustomization.yaml"); err != nil {
			return nil, rollbackOutput{}, err
		}
		msg := fmt.Sprintf("rollback: %v@%s", app.AllImageRepos(), prevTag)
		if err := runner.Commit(msg); err != nil {
			return nil, rollbackOutput{}, err
		}
		if err := runner.Push(); err != nil {
			return nil, rollbackOutput{}, err
		}
		return nil, rollbackOutput{Message: preview + " — commit enviado"}, nil
	}
}

func previousImageTag(runner *git.Runner, kustRel, current string) (string, error) {
	out, err := runner.Run("log", "--format=%H", "--", kustRel)
	if err != nil {
		return "", err
	}
	commits := strings.Split(strings.TrimSpace(out), "\n")
	for _, c := range commits {
		if c == "" {
			continue
		}
		content, err := runner.ShowFileAtCommit(c, kustRel)
		if err != nil {
			continue
		}
		tag := extractTag(content)
		if tag != "" && tag != current {
			return tag, nil
		}
	}
	return "", fmt.Errorf("tag anterior não encontrada para %s", kustRel)
}

func extractTag(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "newTag:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "newTag:"))
		}
	}
	return ""
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

type scaffoldAppOutput struct {
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
	DNS     string `json:"dnsHint,omitempty"`
}

func scaffoldAppHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldAppInput) (*mcp.CallToolResult, scaffoldAppOutput, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldAppInput) (*mcp.CallToolResult, scaffoldAppOutput, error) {
		if in.Name == "" || in.Host == "" {
			return nil, scaffoldAppOutput{}, fmt.Errorf("name e host são obrigatórios")
		}
		p := scaffold.AppParams{
			Name: in.Name, Host: in.Host, ContainerPort: in.ContainerPort,
			CPURequest: in.CPURequest, MemoryRequest: in.MemoryRequest,
			MemoryLimit: in.MemoryLimit, Exposure: in.Exposure,
			ImageRef: appctx.ImageRegistry + "/gabehamasaki/" + in.Name,
		}
		files, err := scaffold.RenderAppFiles(p)
		if err != nil {
			return nil, scaffoldAppOutput{}, err
		}
		msg, err := scaffold.WriteFiles(env.Runtime.InfraRepo, files, in.Write)
		if err != nil && in.Write {
			return nil, scaffoldAppOutput{Content: msg}, err
		}
		return nil, scaffoldAppOutput{Content: msg, Message: msg, DNS: scaffold.DNSHint(p.Exposure)}, nil
	}
}

type scaffoldWorkflowInput struct {
	App   string `json:"app,omitempty"`
	Write bool   `json:"write,omitempty"`
}

type scaffoldWorkflowOutput struct {
	Workflow  string   `json:"workflow,omitempty"`
	Hinfra    string   `json:"hinfra,omitempty"`
	Checklist []string `json:"checklist,omitempty"`
	Message   string   `json:"message,omitempty"`
}

func scaffoldWorkflowHandler(env *Env) func(context.Context, *mcp.CallToolRequest, scaffoldWorkflowInput) (*mcp.CallToolResult, scaffoldWorkflowOutput, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in scaffoldWorkflowInput) (*mcp.CallToolResult, scaffoldWorkflowOutput, error) {
		app, err := resolveApp(env, in.App)
		if err != nil {
			return nil, scaffoldWorkflowOutput{}, err
		}
		tmplPath := filepath.Join(env.Runtime.InfraRepo, "docs", scaffold.WorkflowTemplateName(app.IsMonorepo()))
		tmplBytes, err := os.ReadFile(tmplPath)
		if err != nil {
			return nil, scaffoldWorkflowOutput{}, err
		}
		var workflow string
		if app.IsMonorepo() {
			workflow = scaffold.RenderMonorepoWorkflow(string(tmplBytes), scaffold.MonorepoWorkflowParams{
				AppPath: app.AppPath,
				Images:  app.Images,
			})
		} else {
			workflow = scaffold.RenderWorkflow(string(tmplBytes), scaffold.WorkflowParams{
				ImageName: app.Image,
				AppPath:   app.AppPath,
			})
		}
		hinfra := scaffold.RenderHinfra(scaffold.HinfraParams{
			App: app.Name, Image: app.Image, Images: app.Images, Routing: app.Routing,
			AppPath: app.AppPath, Namespace: app.Namespace, Host: app.Host, Exposure: app.Exposure,
		})
		checklist := buildChecklist(app)
		if !in.Write {
			return nil, scaffoldWorkflowOutput{Workflow: workflow, Hinfra: hinfra, Checklist: checklist}, nil
		}
		files := map[string]string{
			".github/workflows/deploy.yml": workflow,
			"hinfra.yml":                   hinfra,
		}
		msg, err := scaffold.WriteFiles(app.ProjectRepo, files, true)
		if err != nil {
			return nil, scaffoldWorkflowOutput{Workflow: workflow, Hinfra: hinfra, Checklist: checklist, Message: msg}, err
		}
		return nil, scaffoldWorkflowOutput{Message: msg, Checklist: checklist}, nil
	}
}

func buildChecklist(app *appctx.AppContext) []string {
	items := []string{}
	if app.IsMonorepo() {
		for _, component := range []string{"api", "web"} {
			dockerfile := filepath.Join(app.ProjectRepo, component, "Dockerfile")
			if _, err := os.Stat(dockerfile); err != nil {
				items = append(items, fmt.Sprintf("%s/Dockerfile ausente", component))
			} else {
				items = append(items, fmt.Sprintf("%s/Dockerfile presente", component))
			}
		}
	} else {
		dockerfile := filepath.Join(app.ProjectRepo, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			items = append(items, "Dockerfile ausente na raiz do projeto")
		} else {
			items = append(items, "Dockerfile presente")
		}
	}
	kust := filepath.Join(app.InfraRepo, app.AppPath, "kustomization.yaml")
	if _, err := os.Stat(kust); err != nil {
		items = append(items, "apps/"+app.Name+"/kustomization.yaml ausente no repo infra")
	} else {
		items = append(items, "kustomization.yaml presente no repo infra")
	}
	if hasSecret(app) {
		items = append(items, "INFRA_REPO_TOKEN configurado")
	} else {
		owner, repo, _ := git.ParseOriginURL(mustOrigin(app.ProjectRepo))
		items = append(items, fmt.Sprintf("INFRA_REPO_TOKEN ausente — rode: gh secret set INFRA_REPO_TOKEN --repo %s/%s --body \"<PAT>\"", owner, repo))
	}
	return items
}

func hasSecret(app *appctx.AppContext) bool {
	owner, repo, err := git.ParseOriginURL(mustOrigin(app.ProjectRepo))
	if err != nil {
		return false
	}
	cmd := exec.Command("gh", "secret", "list", "--repo", owner+"/"+repo)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "INFRA_REPO_TOKEN")
}

func mustOrigin(projectRepo string) string {
	r := git.NewRunner(projectRepo)
	out, _ := r.OriginURL()
	return out
}
