package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	appctx "github.com/gabehamasaki/infra/tools/mcp/internal/context"
)

func RunSmoke(ctx context.Context, env *Env) error {
	steps := []struct {
		name string
		fn   func() (interface{}, error)
	}{
		{"context", func() (interface{}, error) {
			return resolveApp(env, "")
		}},
		{"deploy_status", func() (interface{}, error) {
			_, out, err := deployStatusHandler(env)(ctx, nil, deployStatusInput{})
			return out, err
		}},
		{"app_health", func() (interface{}, error) {
			_, out, err := appHealthHandler(env)(ctx, nil, appHealthInput{})
			return out, err
		}},
		{"app_logs", func() (interface{}, error) {
			_, out, err := appLogsHandler(env)(ctx, nil, appLogsInput{Tail: 20})
			return out, err
		}},
		{"infra_docs", func() (interface{}, error) {
			_, out, err := infraDocsHandler(env)(ctx, nil, infraDocsInput{Search: "targetRevision"})
			return out, err
		}},
		{"scaffold_workflow", func() (interface{}, error) {
			_, out, err := scaffoldWorkflowHandler(env)(ctx, nil, scaffoldWorkflowInput{Write: false})
			return out, err
		}},
	}

	fmt.Fprintf(os.Stderr, "smoke: cwd=%s infra=%s\n\n", env.CWD, env.Runtime.InfraRepo)
	for _, step := range steps {
		fmt.Fprintf(os.Stderr, "== %s ==\n", step.name)
		out, err := step.fn()
		if err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		fmt.Println()
	}
	return nil
}

func PrintContext(env *Env) (*appctx.AppContext, error) {
	return resolveApp(env, "")
}
