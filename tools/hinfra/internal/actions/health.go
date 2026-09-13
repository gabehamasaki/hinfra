package actions

import (
	"context"
	"fmt"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

type AppHealthResult struct {
	Pods   []k8s.PodSummary `json:"pods"`
	Events []string         `json:"events"`
}

func AppHealth(ctx context.Context, env *Env, appOverride string) (AppHealthResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return AppHealthResult{}, err
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return AppHealthResult{}, err
	}
	pods, err := k8s.ListPods(ctx, kc, app.Namespace)
	if err != nil {
		return AppHealthResult{}, err
	}
	events, err := k8s.ListEvents(ctx, kc, app.Namespace)
	if err != nil {
		return AppHealthResult{}, err
	}
	ev := make([]string, 0, len(events))
	for _, e := range events {
		ev = append(ev, fmt.Sprintf("%s %s: %s", e.Reason, e.InvolvedObject.Name, e.Message))
	}
	return AppHealthResult{Pods: pods, Events: ev}, nil
}
