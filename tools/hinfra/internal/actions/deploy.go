package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/argocd"
	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/k8s"
)

type DeployStatusResult struct {
	Step    int    `json:"step"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	Version string `json:"version,omitempty"`
	SHA     string `json:"sha,omitempty"` // deprecated: same as Version for compat
}

func DeployStatus(ctx context.Context, env *Env, appOverride string) (DeployStatusResult, error) {
	return DeployStatusForEnv(ctx, env, appOverride, "production")
}

func DeployStatusForEnv(ctx context.Context, env *Env, appOverride, deployEnv string) (DeployStatusResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return DeployStatusResult{Step: 0, OK: false, Detail: err.Error()}, nil
	}
	appPath := productionAppPath(app, deployEnv)
	version, err := appctx.ImageTagForAppPath(app.InfraRepo, appPath)
	if err != nil {
		return DeployStatusResult{Step: 1, OK: false, Detail: err.Error()}, nil
	}
	infraGit := git.NewRunner(app.InfraRepo)
	logOut, err := infraGit.LogGrep(version, 5)
	if err != nil || logOut == "" {
		detail := "commit de bump não encontrado no repo infra para versão " + version
		if err != nil {
			detail = err.Error()
		}
		return withVersion(DeployStatusResult{Step: 2, OK: false, Detail: detail}, version), nil
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return withVersion(DeployStatusResult{Step: 3, OK: false, Detail: err.Error()}, version), nil
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return withVersion(DeployStatusResult{Step: 3, OK: false, Detail: err.Error()}, version), nil
	}
	argocdAppName := app.Name
	if deployEnv == "dev" {
		argocdAppName = app.Name + "-dev"
	} else if deployEnv == "homolog" {
		argocdAppName = app.Name + "-homolog"
	}
	st, err := ac.GetApplication(ctx, argocdAppName)
	if err != nil && deployEnv == "production" {
		st, err = ac.GetApplication(ctx, app.Name)
	}
	if err != nil {
		return withVersion(DeployStatusResult{Step: 3, OK: false, Detail: err.Error()}, version), nil
	}
	if st.Sync != "Synced" || st.Health != "Healthy" {
		detail := argocd.FormatAppStatus(st)
		detail += deployStatusHints(st)
		return withVersion(DeployStatusResult{Step: 3, OK: false, Detail: detail}, version), nil
	}
	ns := app.Namespace
	if deployEnv == "dev" {
		ns = app.Name + "-dev"
	} else if deployEnv == "homolog" {
		ns = app.Name + "-hg"
	}
	pods, err := k8s.ListPods(ctx, kc, ns)
	if err != nil {
		return withVersion(DeployStatusResult{Step: 4, OK: false, Detail: err.Error()}, version), nil
	}
	podImages := make([]string, 0, len(pods))
	for _, p := range pods {
		podImages = append(podImages, p.Image)
	}
	if !appctx.ImagesDeployed(podImages, app.AllImageRefs(), version) {
		return withVersion(DeployStatusResult{
			Step: 4, OK: false,
			Detail: fmt.Sprintf("pod(s) não rodam todas as imagens com versão %s; imagens atuais: %v; esperadas: %v", version, podImages, app.AllImageRefs()),
		}, version), nil
	}
	return withVersion(DeployStatusResult{Step: 4, OK: true, Detail: "deploy completo"}, version), nil
}

func productionAppPath(app *appctx.AppContext, deployEnv string) string {
	if app.Hinfra != nil {
		envs := app.Hinfra.EffectiveEnvironments()
		if e, ok := envs[deployEnv]; ok && e.AppPath != "" {
			return e.AppPath
		}
		if deployEnv == "production" {
			return app.Hinfra.ProductionEnvironment().AppPath
		}
	}
	if deployEnv == "dev" {
		return app.AppPath + "/overlays/dev"
	}
	if deployEnv == "homolog" {
		return app.AppPath + "/overlays/homolog"
	}
	return app.AppPath
}

func withVersion(r DeployStatusResult, version string) DeployStatusResult {
	r.Version = version
	r.SHA = version
	return r
}

func deployStatusHints(st *argocd.ApplicationStatus) string {
	var hints []string
	if st.Sync == "OutOfSync" {
		hints = append(hints, "aguarde sync automático ou sync manual no Argo")
	}
	if st.Health == "Progressing" {
		hints = append(hints, "estado transitório (migration/sync) — repita deploy status em alguns minutos")
	}
	if st.Sync != "Synced" {
		hints = append(hints, "sync preso? termine operação antiga no UI do ArgoCD")
	}
	hints = append(hints, "Application novo no infra exige: hinfra argocd refresh --root")
	return "; " + strings.Join(hints, "; ")
}
