package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Runner struct {
	Dir string
}

func NewRunner(dir string) *Runner {
	return &Runner{Dir: dir}
}

func (r *Runner) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (r *Runner) TopLevel() (string, error) {
	return r.Run("rev-parse", "--show-toplevel")
}

func (r *Runner) OriginURL() (string, error) {
	return r.Run("remote", "get-url", "origin")
}

func (r *Runner) LSRemote(ref string) (string, error) {
	out, err := r.Run("ls-remote", "origin", ref)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("ref %s não encontrada em origin", ref)
}

func (r *Runner) PullRebase() error {
	_, err := r.Run("pull", "--rebase")
	return err
}

func (r *Runner) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := r.Run(args...)
	return err
}

func (r *Runner) Commit(message string) error {
	_, err := r.Run("commit", "-m", message)
	return err
}

func (r *Runner) Push() error {
	_, err := r.Run("push")
	return err
}

func (r *Runner) LogGrep(pattern string, limit int) (string, error) {
	return r.Run("log", "--grep="+pattern, "--oneline", "-n", fmt.Sprintf("%d", limit))
}

func (r *Runner) ShowFileAtCommit(commit, path string) (string, error) {
	return r.Run("show", commit+":"+path)
}

func ParseOriginURL(url string) (owner, repo string, err error) {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")

	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(url, "git@"), ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("origin SSH inválida: %s", url)
		}
		return parseOwnerRepo(parts[1])
	}

	if idx := strings.Index(url, "://"); idx >= 0 {
		url = url[idx+3:]
	}
	if idx := strings.Index(url, "@"); idx >= 0 {
		url = url[idx+1:]
	}
	return parseOwnerRepo(url)
}

func parseOwnerRepo(path string) (string, string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("origin inválida: %s", path)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}
