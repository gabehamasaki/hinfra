package actions

import (
	"context"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

type ArgoCDRefreshResult struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

func ArgoCDRefresh(ctx context.Context, env *Env, appOverride string) (ArgoCDRefreshResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	path, hasChart, err := ac.SourcePath(ctx, app.Name)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	target := argocd.RefreshTarget(app.Name, path, hasChart)
	if err := ac.Refresh(ctx, target); err != nil {
		return ArgoCDRefreshResult{}, err
	}
	return ArgoCDRefreshResult{Target: target, Message: "refresh hard aplicado"}, nil
}

func ArgoCDRefreshByName(ctx context.Context, env *Env, appName string) (ArgoCDRefreshResult, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return ArgoCDRefreshResult{}, err
	}
	if err := ac.Refresh(ctx, appName); err != nil {
		return ArgoCDRefreshResult{}, err
	}
	return ArgoCDRefreshResult{Target: appName, Message: "refresh hard aplicado"}, nil
}

func ArgoCDRefreshRoot(ctx context.Context, env *Env) (ArgoCDRefreshResult, error) {
	return ArgoCDRefreshByName(ctx, env, "root-app")
}
