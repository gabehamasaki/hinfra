package actions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

type ArgoCDRefreshResult struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

type ArgoCDSyncInput struct {
	AppOverride string
	Root        bool
	Projects    bool
	Force       bool
	Refresh     bool
	Wait        bool
}

type ArgoCDSyncResult struct {
	Targets []string `json:"targets"`
	Message string   `json:"message"`
}

func argocdClient(ctx context.Context, env *Env) (*argocd.Client, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return nil, err
	}
	return argocd.NewClient(kc.Config)
}

func syncOne(ctx context.Context, ac *argocd.Client, name string, in ArgoCDSyncInput) error {
	if in.Refresh {
		if err := ac.Refresh(ctx, name); err != nil {
			return err
		}
	}
	if err := ac.RequestSync(ctx, name, argocd.SyncRequest{Force: in.Force}); err != nil {
		return err
	}
	if in.Wait {
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if err := ac.WaitForSync(waitCtx, name); err != nil {
			return err
		}
	}
	return nil
}

func ArgoCDSync(ctx context.Context, env *Env, in ArgoCDSyncInput) (ArgoCDSyncResult, error) {
	ac, err := argocdClient(ctx, env)
	if err != nil {
		return ArgoCDSyncResult{}, err
	}

	var targets []string
	if in.Root {
		targets = []string{env.Runtime.ArgocdRootApplication()}
	} else if in.Projects {
		rows, err := ac.ListApplications(ctx)
		if err != nil {
			return ArgoCDSyncResult{}, err
		}
		for _, row := range rows {
			if row.Layer != argocd.LayerProjects || row.Name == "example" {
				continue
			}
			targets = append(targets, row.Name)
		}
		if len(targets) == 0 {
			return ArgoCDSyncResult{}, fmt.Errorf("nenhum Application de projeto encontrado no Argo CD")
		}
	} else {
		app, err := ResolveApp(env, in.AppOverride)
		if err != nil {
			return ArgoCDSyncResult{}, err
		}
		targets = []string{app.Name}
	}

	for _, name := range targets {
		if err := syncOne(ctx, ac, name, in); err != nil {
			return ArgoCDSyncResult{}, fmt.Errorf("%s: %w", name, err)
		}
	}

	msg := fmt.Sprintf("sync disparado em %s", strings.Join(targets, ", "))
	if in.Wait {
		msg = fmt.Sprintf("sync concluído em %s", strings.Join(targets, ", "))
	}
	return ArgoCDSyncResult{Targets: targets, Message: msg}, nil
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
	target := argocd.RefreshTarget(app.Name, path, hasChart, env.Runtime.ArgocdRootApplication())
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
	return ArgoCDRefreshByName(ctx, env, env.Runtime.ArgocdRootApplication())
}
