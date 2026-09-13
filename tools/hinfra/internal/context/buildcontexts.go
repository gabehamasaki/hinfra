package context

// BuildContext describes Docker build context and Dockerfile path for CI.
// File is always relative to the repository root, not to Context.
type BuildContext struct {
	Context string `yaml:"context"`
	File    string `yaml:"file"`
}

// DefaultBuildContexts returns conventional api/web layout (./api, ./web).
func DefaultBuildContexts() map[string]BuildContext {
	return map[string]BuildContext{
		"api": {Context: "./api", File: "api/Dockerfile"},
		"web": {Context: "./web", File: "web/Dockerfile"},
	}
}

// BackendFrontendBuildContexts is the backend/docker + frontend layout.
func BackendFrontendBuildContexts() map[string]BuildContext {
	return map[string]BuildContext{
		"api": {Context: "./backend", File: "backend/docker/Dockerfile"},
		"web": {Context: "./frontend", File: "frontend/Dockerfile"},
	}
}

// MergeBuildContexts overlays hinfra.yml values on defaults for api and web.
func MergeBuildContexts(fromHinfra map[string]BuildContext) map[string]BuildContext {
	out := DefaultBuildContexts()
	for key, custom := range fromHinfra {
		base, ok := out[key]
		if !ok {
			base = BuildContext{}
		}
		if custom.Context != "" {
			base.Context = custom.Context
		}
		if custom.File != "" {
			base.File = custom.File
		}
		out[key] = base
	}
	return out
}

// ResolvedBuildContexts returns build contexts for a monorepo app.
func ResolvedBuildContexts(hinfra *HinfraConfig) map[string]BuildContext {
	if hinfra == nil || len(hinfra.Images) == 0 {
		return nil
	}
	if len(hinfra.BuildContexts) == 0 {
		return DefaultBuildContexts()
	}
	return MergeBuildContexts(hinfra.BuildContexts)
}
