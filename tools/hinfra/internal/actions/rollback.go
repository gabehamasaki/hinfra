package actions

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	appctx "github.com/gabehamasaki/hinfra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/git"
)

type RollbackResult struct {
	Preview string `json:"preview,omitempty"`
	Message string `json:"message,omitempty"`
}

func Rollback(env *Env, appOverride string, confirm bool) (RollbackResult, error) {
	app, err := ResolveApp(env, appOverride)
	if err != nil {
		return RollbackResult{}, err
	}
	runner := git.NewRunner(app.InfraRepo)
	if err := runner.PullRebase(); err != nil {
		return RollbackResult{}, err
	}
	kustPath := filepath.Join(app.InfraRepo, app.AppPath, "kustomization.yaml")
	_, currentTag, err := appctx.ImageFromKustomization(kustPath)
	if err != nil {
		return RollbackResult{}, err
	}
	prevTag, err := previousImageTag(runner, app.AppPath+"/kustomization.yaml", currentTag)
	if err != nil {
		return RollbackResult{}, err
	}
	preview := fmt.Sprintf("rollback de %s para %s em %s", currentTag, prevTag, kustPath)
	if !confirm {
		return RollbackResult{Preview: preview + " — passe --confirm para executar"}, nil
	}
	appDir := filepath.Join(app.InfraRepo, app.AppPath)
	for _, ref := range app.AllImageRefs() {
		cmd := exec.Command("kustomize", "edit", "set", "image", ref+"="+ref+":"+prevTag)
		cmd.Dir = appDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return RollbackResult{}, fmt.Errorf("kustomize (%s): %s: %w", ref, out, err)
		}
	}
	if err := runner.Add(app.AppPath + "/kustomization.yaml"); err != nil {
		return RollbackResult{}, err
	}
	msg := fmt.Sprintf("rollback: %v@%s", app.AllImageRepos(), prevTag)
	if err := runner.Commit(msg); err != nil {
		return RollbackResult{}, err
	}
	if err := runner.Push(); err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{Message: preview + " — commit enviado"}, nil
}

func previousImageTag(runner *git.Runner, kustRel, current string) (string, error) {
	out, err := runner.Run("log", "--format=%H", "--", kustRel)
	if err != nil {
		return "", err
	}
	commits := strings.Split(strings.TrimSpace(out), "\n")
	for _, c := range commits {
		if c == "" {
			continue
		}
		content, err := runner.ShowFileAtCommit(c, kustRel)
		if err != nil {
			continue
		}
		tag := extractTag(content)
		if tag != "" && tag != current {
			return tag, nil
		}
	}
	return "", fmt.Errorf("tag anterior não encontrada para %s", kustRel)
}

func extractTag(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "newTag:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "newTag:"))
		}
	}
	return ""
}
