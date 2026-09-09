package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
)

func RunSmoke(ctx context.Context, env *Env) error {
	steps := []struct {
		name string
		fn   func() (interface{}, error)
	}{
		{"context", func() (interface{}, error) {
			return actions.ResolveApp(env, "")
		}},
		{"deploy_status", func() (interface{}, error) {
			return actions.DeployStatus(ctx, env, "")
		}},
		{"app_health", func() (interface{}, error) {
			return actions.AppHealth(ctx, env, "")
		}},
		{"app_logs", func() (interface{}, error) {
			return actions.AppLogs(ctx, env, "", actions.AppLogsInput{Tail: 20})
		}},
		{"infra_docs", func() (interface{}, error) {
			return actions.InfraDocs(env, "targetRevision", "")
		}},
		{"scaffold_workflow", func() (interface{}, error) {
			return actions.ScaffoldWorkflow(env, "", false)
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
