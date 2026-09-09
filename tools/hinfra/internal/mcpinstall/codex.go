package mcpinstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func installCodex(bin string) string {
	path := filepath.Join(os.Getenv("HOME"), ".codex", "config.toml")
	var root map[string]interface{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = toml.Unmarshal(raw, &root)
	}
	if root == nil {
		root = map[string]interface{}{}
	}
	servers, _ := root["mcp_servers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers[serverName] = map[string]interface{}{
		"command": bin,
		"args":    []string{"mcp"},
	}
	root["mcp_servers"] = servers
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Sprintf("✗ Codex: %v", err)
	}
	out, err := toml.Marshal(root)
	if err != nil {
		return fmt.Sprintf("✗ Codex: %v", err)
	}
	// pelletier may not preserve table structure from map — ensure valid TOML block
	content := string(out)
	if !strings.Contains(content, "[mcp_servers.") {
		block := fmt.Sprintf("\n[mcp_servers.%s]\ncommand = \"%s\"\nargs = [\"mcp\"]\n", serverName, bin)
		if _, err := os.Stat(path); err != nil {
			content = block
		} else {
			content = content + block
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("✗ Codex: %v", err)
	}
	return fmt.Sprintf("✓ Codex    → %s", path)
}
