package actions

import (
	"context"
	"fmt"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/argocd"
	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/k8s"
)

func ListApplications(ctx context.Context, env *Env) ([]argocd.ApplicationRow, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return nil, err
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return nil, err
	}
	rows, err := ac.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Layer != argocd.LayerProjects {
			continue
		}
		tag, err := appctx.ProductionImageTag(env.Runtime.InfraRepo, rows[i].Name)
		if err == nil {
			rows[i].Version = tag
		}
	}
	return rows, nil
}

func AppHealthByName(ctx context.Context, env *Env, namespace string) (AppHealthResult, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return AppHealthResult{}, err
	}
	pods, err := k8s.ListPods(ctx, kc, namespace)
	if err != nil {
		return AppHealthResult{}, err
	}
	events, err := k8s.ListEvents(ctx, kc, namespace)
	if err != nil {
		return AppHealthResult{}, err
	}
	ev := make([]string, 0, len(events))
	for _, e := range events {
		ev = append(ev, e.Reason+" "+e.InvolvedObject.Name+": "+e.Message)
	}
	return AppHealthResult{Pods: pods, Events: ev}, nil
}

func AppLogsByNamespace(ctx context.Context, env *Env, namespace string, in AppLogsInput) (AppLogsResult, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return AppLogsResult{}, err
	}
	pod := in.Pod
	if pod == "" {
		pods, err := k8s.ListPods(ctx, kc, namespace)
		if err != nil {
			return AppLogsResult{}, err
		}
		if len(pods) == 0 {
			return AppLogsResult{}, fmt.Errorf("nenhum pod em namespace %s", namespace)
		}
		pod = pods[0].Name
	}
	tail := int64(100)
	if in.Tail > 0 {
		tail = int64(in.Tail)
	}
	logs, err := k8s.GetPodLogs(ctx, kc, namespace, pod, in.Container, in.Previous, tail)
	if err != nil {
		return AppLogsResult{}, err
	}
	return AppLogsResult{Logs: logs, Pod: pod}, nil
}
