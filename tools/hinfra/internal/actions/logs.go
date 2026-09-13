package actions

import (
	"context"
	"fmt"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

type AppLogsInput struct {
	Pod       string
	Container string
	Previous  bool
	Tail      int
}

type AppLogsResult struct {
	Logs string `json:"logs"`
	Pod  string `json:"pod"`
}

func AppLogs(ctx context.Context, env *Env, appOverride string, in AppLogsInput) (AppLogsResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return AppLogsResult{}, err
	}
	kc, err := k8s.NewClient(ctx, app.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return AppLogsResult{}, err
	}
	pod := in.Pod
	if pod == "" {
		pods, err := k8s.ListPods(ctx, kc, app.Namespace)
		if err != nil {
			return AppLogsResult{}, err
		}
		if len(pods) == 0 {
			return AppLogsResult{}, fmt.Errorf("nenhum pod em namespace %s", app.Namespace)
		}
		pod = pods[0].Name
	}
	tail := int64(100)
	if in.Tail > 0 {
		tail = int64(in.Tail)
	}
	logs, err := k8s.GetPodLogs(ctx, kc, app.Namespace, pod, in.Container, in.Previous, tail)
	if err != nil {
		return AppLogsResult{}, err
	}
	return AppLogsResult{Logs: logs, Pod: pod}, nil
}
