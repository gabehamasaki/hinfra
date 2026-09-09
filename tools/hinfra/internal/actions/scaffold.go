package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/scaffold"
)

type ScaffoldAppInput struct {
	Name          string
	Host          string
	ContainerPort int
	CPURequest    string
	MemoryRequest string
	MemoryLimit   string
	Exposure      string
	Write         bool
}

type ScaffoldAppResult struct {
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
	DNS     string `json:"dnsHint,omitempty"`
}

func ScaffoldApp(env *Env, in ScaffoldAppInput) (ScaffoldAppResult, error) {
	if in.Name == "" || in.Host == "" {
		return ScaffoldAppResult{}, fmt.Errorf("name e host são obrigatórios")
	}
	p := scaffold.AppParams{
		Name: in.Name, Host: in.Host, ContainerPort: in.ContainerPort,
		CPURequest: in.CPURequest, MemoryRequest: in.MemoryRequest,
		MemoryLimit: in.MemoryLimit, Exposure: in.Exposure,
		ImageRef: appctx.ImageRegistry + "/gabehamasaki/" + in.Name,
	}
	files, err := scaffold.RenderAppFiles(p)
	if err != nil {
		return ScaffoldAppResult{}, err
	}
	msg, err := scaffold.WriteFiles(env.Runtime.InfraRepo, files, in.Write)
	if err != nil && in.Write {
		return ScaffoldAppResult{Content: msg}, err
	}
	return ScaffoldAppResult{Content: msg, Message: msg, DNS: scaffold.DNSHint(p.Exposure)}, nil
}

type ScaffoldWorkflowResult struct {
	Workflow  string   `json:"workflow,omitempty"`
	Hinfra    string   `json:"hinfra,omitempty"`
	Checklist []string `json:"checklist,omitempty"`
	Message   string   `json:"message,omitempty"`
}

func ScaffoldWorkflow(env *Env, appOverride string, write bool) (ScaffoldWorkflowResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return ScaffoldWorkflowResult{}, err
	}
	tmplPath := filepath.Join(env.Runtime.InfraRepo, "docs", scaffold.WorkflowTemplateName(app.IsMonorepo()))
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return ScaffoldWorkflowResult{}, err
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
	if !write {
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
