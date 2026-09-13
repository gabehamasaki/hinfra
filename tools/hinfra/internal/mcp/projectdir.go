package mcp

// withProjectDir returns a copy of env with ProjectDir set when the MCP cwd is not the project repo.
func withProjectDir(env *Env, projectDir string) *Env {
	if projectDir == "" {
		return env
	}
	clone := *env
	clone.ProjectDir = projectDir
	return &clone
}
