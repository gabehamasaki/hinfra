package actions

import (
	"context"
	"fmt"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/argocd"
	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/k8s"
)

type DeployStatusResult struct {
	Step   int    `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	SHA    string `json:"sha,omitempty"`
}

func DeployStatus(ctx context.Context, env *Env, appOverride string) (DeployStatusResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return DeployStatusResult{Step: 0, OK: false, Detail: err.Error()}, nil
	}
	runner := git.NewRunner(app.ProjectRepo)
	sha, err := runner.LSRemote("refs/heads/main")
	if err != nil {
		return DeployStatusResult{Step: 1, OK: false, Detail: err.Error()}, nil
	}
	infraGit := git.NewRunner(app.InfraRepo)
	logOut, err := infraGit.LogGrep(sha, 5)
	if err != nil || logOut == "" {
		detail := "commit de bump não encontrado no repo infra para SHA " + sha
		if err != nil {
			detail = err.Error()
		}
		return DeployStatusResult{Step: 2, OK: false, Detail: detail, SHA: sha}, nil
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return DeployStatusResult{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return DeployStatusResult{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
	}
	st, err := ac.GetApplication(ctx, app.Name)
	if err != nil {
		return DeployStatusResult{Step: 3, OK: false, Detail: err.Error(), SHA: sha}, nil
	}
	if st.Sync != "Synced" || st.Health != "Healthy" {
		return DeployStatusResult{Step: 3, OK: false, Detail: argocd.FormatAppStatus(st), SHA: sha}, nil
	}
	pods, err := k8s.ListPods(ctx, kc, app.Namespace)
	if err != nil {
		return DeployStatusResult{Step: 4, OK: false, Detail: err.Error(), SHA: sha}, nil
	}
	podImages := make([]string, 0, len(pods))
	for _, p := range pods {
		podImages = append(podImages, p.Image)
	}
	if !appctx.ImagesDeployed(podImages, app.AllImageRefs(), sha) {
		return DeployStatusResult{
			Step: 4, OK: false, SHA: sha,
			Detail: fmt.Sprintf("pod(s) não rodam todas as imagens com SHA %s; imagens atuais: %v; esperadas: %v", sha, podImages, app.AllImageRefs()),
		}, nil
	}
	return DeployStatusResult{Step: 4, OK: true, Detail: "deploy completo", SHA: sha}, nil
}
