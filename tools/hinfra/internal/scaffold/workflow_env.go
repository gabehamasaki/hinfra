package scaffold

import (
	"fmt"
	"sort"
	"strings"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
)

const (
	workflowPlaceholderTagTriggers = "TAG_TRIGGER_YAML"
	workflowPlaceholderBranchMap   = "DEPLOY_BRANCH_MAP"
)

func ApplyWorkflowEnvironments(templateContent string, cfg *appctx.HinfraConfig) string {
	out := templateContent
	triggers := defaultTagTriggersYAML()
	branchMap := defaultBranchMapYAML()
	if cfg != nil {
		if lines := enabledTagTriggersYAML(cfg); len(lines) > 0 {
			triggers = lines
		}
		if m := branchMapFromConfig(cfg); m != "" {
			branchMap = m
		}
	}
	out = strings.ReplaceAll(out, workflowPlaceholderTagTriggers, triggers)
	out = strings.ReplaceAll(out, workflowPlaceholderBranchMap, branchMap)
	return out
}

func defaultTagTriggersYAML() string {
	return `      - "v*"
      - "dev/**"
      - "hg/**"`
}

func enabledTagTriggersYAML(cfg *appctx.HinfraConfig) string {
	triggers := cfg.EnabledTagTriggers()
	if len(triggers) == 0 {
		return ""
	}
	sort.Strings(triggers)
	var b strings.Builder
	for _, t := range triggers {
		b.WriteString(fmt.Sprintf("      - \"%s\"\n", t))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func defaultBranchMapYAML() string {
	return `          v*) deploy_branch=main ;;
          dev/*) deploy_branch=main ;;
          hg/*) deploy_branch=main ;;`
}

func branchMapFromConfig(cfg *appctx.HinfraConfig) string {
	envs := cfg.EffectiveEnvironments()
	type row struct {
		pattern string
		branch  string
	}
	var rows []row
	for _, env := range envs {
		if !env.Enabled {
			continue
		}
		pat := tagPrefixShellPattern(env.TagPrefix)
		if pat == "" {
			continue
		}
		rows = append(rows, row{pattern: pat, branch: env.EnvironmentBranch()})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		return len(rows[i].pattern) > len(rows[j].pattern)
	})
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("          %s) deploy_branch=%s ;;\n", r.pattern, r.branch))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func tagPrefixShellPattern(prefix string) string {
	switch prefix {
	case "v", "v*":
		return "v*)"
	case "dev/", "dev/*":
		return "dev/*)"
	case "hg/", "hg/*":
		return "hg/*)"
	default:
		if strings.HasSuffix(prefix, "/") {
			return prefix + "*)"
		}
		return prefix + "*)"
	}
}
