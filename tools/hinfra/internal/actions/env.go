package actions

import (
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
)

type Env struct {
	Runtime *config.Runtime
	CWD     string
}

func ResolveApp(env *Env, appOverride string) (*appctx.AppContext, error) {
	resolver := appctx.NewResolver(env.Runtime.InfraRepo, env.Runtime.Kubeconfig)
	return resolver.Resolve(env.CWD, appOverride)
}
