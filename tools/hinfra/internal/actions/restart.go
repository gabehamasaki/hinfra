package actions

import (
	"context"
	"fmt"

	appctx "github.com/gabehamasaki/hinfra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

type AppRestartResult struct {
	Message string `json:"message"`
}

func AppRestart(ctx context.Context, env *Env, appOverride string) (AppRestartResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return AppRestartResult{}, err
	}
	if !appctx.AppDirExists(app.InfraRepo, app.Namespace) {
		return AppRestartResult{}, fmt.Errorf("namespace %s não corresponde a apps/ no repo infra", app.Namespace)
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return AppRestartResult{}, err
	}
	if err := k8s.RestartDeployment(ctx, kc, app.Namespace, app.Name); err != nil {
		return AppRestartResult{}, err
	}
	return AppRestartResult{Message: fmt.Sprintf("rollout restart em deployment/%s namespace %s", app.Name, app.Namespace)}, nil
}
