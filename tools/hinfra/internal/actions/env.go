package actions

import (
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
)

type Env struct {
	Runtime    *config.Runtime
	CWD        string
	ProjectDir string // optional: project repo root when MCP cwd is infra
}

func ResolveApp(env *Env, appOverride string) (*appctx.AppContext, error) {
	resolver := appctx.NewResolver(env.Runtime.InfraRepo, env.Runtime.Kubeconfig)
	cwd := env.CWD
	if env.ProjectDir != "" {
		cwd = env.ProjectDir
	}
	return resolver.Resolve(cwd, appOverride)
}
