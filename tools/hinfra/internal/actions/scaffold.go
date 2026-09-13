package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appctx "github.com/gabehamasaki/hinfra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/scaffold"
)

type ScaffoldAppInput struct {
	Name              string
	Host              string
	ContainerPort     int
	CPURequest        string
	MemoryRequest     string
	MemoryLimit       string
	Exposure          string
	Write             bool
	Monorepo          bool
	ApiPort           int
	WebPort           int
	NeedsMigration    bool
	NeedsSealedSecret bool
	MigrationCommand  string
	Environments      map[string]appctx.HinfraEnvironment
}

type ScaffoldAppResult struct {
	Content   string   `json:"content,omitempty"`
	Message   string   `json:"message,omitempty"`
	DNS       string   `json:"dnsHint,omitempty"`
	Checklist []string `json:"checklist,omitempty"`
}

func ScaffoldApp(env *Env, in ScaffoldAppInput) (ScaffoldAppResult, error) {
	if in.Name == "" || in.Host == "" {
		return ScaffoldAppResult{}, fmt.Errorf("name e host são obrigatórios")
	}
	exposure := in.Exposure
	if exposure == "" {
		exposure = "public"
	}
	var files map[string]string
	var err error
	if in.Monorepo {
		p := scaffold.MonorepoAppParams{
			Name: in.Name, Host: in.Host, Exposure: exposure,
			ApiPort: in.ApiPort, WebPort: in.WebPort,
			NeedsMigration:    true,
			NeedsSealedSecret: true,
			MigrationCommand:  in.MigrationCommand,
			CPURequest:        in.CPURequest, MemoryRequest: in.MemoryRequest, MemoryLimit: in.MemoryLimit,
		}
		files, err = scaffold.RenderMonorepoAppFiles(p)
	} else {
		p := scaffold.AppParams{
			Name: in.Name, Host: in.Host, ContainerPort: in.ContainerPort,
			CPURequest: in.CPURequest, MemoryRequest: in.MemoryRequest,
			MemoryLimit: in.MemoryLimit, Exposure: exposure,
			ImageRef: appctx.ImageRegistry + "/gabehamasaki/" + in.Name,
		}
		files, err = scaffold.RenderAppFiles(p)
	}
	if err != nil {
		return ScaffoldAppResult{}, err
	}
	if len(in.Environments) > 0 {
		imageRefs := []string{appctx.ImageRegistry + "/gabehamasaki/" + in.Name}
		if in.Monorepo {
			imageRefs = []string{
				appctx.ImageRegistry + "/gabehamasaki/" + in.Name + "-api",
				appctx.ImageRegistry + "/gabehamasaki/" + in.Name + "-web",
			}
		}
		scaffold.AppendEnvironmentOverlayFiles(files, in.Name, imageRefs, in.Environments)
	}
	msg, err := scaffold.WriteFiles(env.Runtime.AppsRepo(), files, in.Write)
	checklist := scaffoldAppChecklist(in.Monorepo, in.Write)
	if err != nil && in.Write {
		return ScaffoldAppResult{Content: msg, Checklist: checklist}, err
	}
	return ScaffoldAppResult{Content: msg, Message: msg, DNS: scaffold.DNSHint(exposure), Checklist: checklist}, nil
}

func scaffoldAppChecklist(monorepo, wrote bool) []string {
	items := []string{}
	if wrote {
		items = append(items, "hinfra argocd refresh --root (registrar Application novo no ArgoCD)")
	}
	if monorepo {
		items = append(items, "Gere sealed-secret.yaml com hinfra seal secret e adicione ao kustomization")
		items = append(items, "Ajuste migration-job.yaml command para o migrator real da API")
	}
	return items
}

type ScaffoldWorkflowResult struct {
	Workflow  string   `json:"workflow,omitempty"`
	Hinfra    string   `json:"hinfra,omitempty"`
	Checklist []string `json:"checklist,omitempty"`
	Message   string   `json:"message,omitempty"`
}

type ScaffoldWorkflowInput struct {
	AppOverride string
	Write         bool
	Monorepo      bool
}

func ScaffoldWorkflow(env *Env, in ScaffoldWorkflowInput) (ScaffoldWorkflowResult, error) {
	app, err := ResolveApp(env, in.AppOverride)
	if err != nil {
		return ScaffoldWorkflowResult{}, err
	}
	if in.Monorepo && !app.IsMonorepo() {
		owner, _, parseErr := git.ParseOriginURL(mustOrigin(app.ProjectRepo))
		if parseErr != nil {
			return ScaffoldWorkflowResult{}, parseErr
		}
		appName := app.Name
		if in.AppOverride != "" {
			appName = in.AppOverride
		}
		app.Images = map[string]string{
			"api": owner + "/" + appName + "-api",
			"web": owner + "/" + appName + "-web",
		}
		app.BuildContexts = appctx.DefaultBuildContexts()
		if app.Routing == nil {
			app.Routing = &appctx.HinfraRouting{ApiPath: "/api", WebPath: "/"}
		}
	}
	tmplPath := filepath.Join(env.Runtime.InfraRepo, "docs", scaffold.WorkflowTemplateName(app.IsMonorepo()))
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return ScaffoldWorkflowResult{}, err
	}
	var workflow string
	if app.IsMonorepo() {
		bc := app.BuildContexts
		if len(bc) == 0 {
			bc = appctx.DefaultBuildContexts()
		}
		workflow = scaffold.RenderMonorepoWorkflow(string(tmplBytes), scaffold.MonorepoWorkflowParams{
			AppPath:       app.AppPath,
			Images:        app.Images,
			BuildContexts: bc,
		}, workflowHinfraConfig(app))
	} else {
		workflow = scaffold.RenderWorkflow(string(tmplBytes), scaffold.WorkflowParams{
			ImageName: app.Image,
			AppPath:   app.AppPath,
		}, workflowHinfraConfig(app))
	}
	bc := app.BuildContexts
	if app.IsMonorepo() && len(bc) == 0 {
		bc = appctx.DefaultBuildContexts()
	}
	envs := appctx.DefaultEnvironments(app.Name)
	if app.Hinfra != nil && len(app.Hinfra.Environments) > 0 {
		envs = app.Hinfra.Environments
	}
	hinfra := scaffold.RenderHinfra(scaffold.HinfraParams{
		App: app.Name, Image: app.Image, Images: app.Images, BuildContexts: bc, Routing: app.Routing,
		AppPath: app.AppPath, Namespace: app.Namespace, Host: app.Host, Exposure: app.Exposure,
		Environments: envs,
	})
	checklist := buildChecklist(app)
	if !in.Write {
		return ScaffoldWorkflowResult{Workflow: workflow, Hinfra: hinfra, Checklist: checklist}, nil
	}
	files := map[string]string{
		".github/workflows/deploy.yml": workflow,
		"hinfra.yml":                   hinfra,
	}
	msg, err := scaffold.WriteFiles(app.ProjectRepo, files, true)
	if err != nil {
		return ScaffoldWorkflowResult{Workflow: workflow, Hinfra: hinfra, Checklist: checklist, Message: msg}, err
	}
	return ScaffoldWorkflowResult{Message: msg, Checklist: checklist}, nil
}

func buildChecklist(app *appctx.AppContext) []string {
	items := []string{}
	if app.IsMonorepo() {
		bc := app.BuildContexts
		if len(bc) == 0 {
			bc = appctx.DefaultBuildContexts()
		}
		for _, component := range []string{"api", "web"} {
			ctx, ok := bc[component]
			if !ok {
				continue
			}
			dockerPath := filepath.Join(app.ProjectRepo, ctx.File)
			if _, err := os.Stat(dockerPath); err != nil {
				items = append(items, fmt.Sprintf("Dockerfile ausente: %s (buildContexts.%s.file)", ctx.File, component))
			} else {
				items = append(items, fmt.Sprintf("Dockerfile presente: %s", ctx.File))
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
		items = append(items, "HINFRA_WORKLOADS_TOKEN configurado")
	} else {
		owner, repo, _ := git.ParseOriginURL(mustOrigin(app.ProjectRepo))
		items = append(items, fmt.Sprintf("HINFRA_WORKLOADS_TOKEN ausente — rode: gh secret set HINFRA_WORKLOADS_TOKEN --repo %s/%s --body \"<PAT>\"", owner, repo))
	}
	items = append(items, "Pacotes GHCR: tornar públicos (Package settings) ou configurar imagePullSecret no namespace")
	items = append(items, "Após gravar Application no infra: hinfra argocd refresh --root")
	items = append(items, "Postgres compartilhado: criar role/DB — ver docs/08-data-services.md (infra_docs read 08-data-services.md)")
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
	return strings.Contains(string(out), "HINFRA_WORKLOADS_TOKEN")
}

func workflowHinfraConfig(app *appctx.AppContext) *appctx.HinfraConfig {
	if app.Hinfra != nil {
		return app.Hinfra
	}
	return &appctx.HinfraConfig{
		App:          app.Name,
		AppPath:      app.AppPath,
		Namespace:    app.Namespace,
		Environments: appctx.DefaultEnvironments(app.Name),
	}
}

func mustOrigin(projectRepo string) string {
	r := git.NewRunner(projectRepo)
	out, _ := r.OriginURL()
	return out
}
