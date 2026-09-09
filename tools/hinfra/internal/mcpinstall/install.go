package mcpinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serverName = "infra"

type Selection struct {
	Cursor   bool
	Claude   bool
	Codex    bool
	OpenCode bool
}

func (s Selection) Any() bool {
	return s.Cursor || s.Claude || s.Codex || s.OpenCode
}

func binPath() string {
	if p, err := exec.LookPath("hinfra"); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "bin", "hinfra")
}

func Install(sel Selection) error {
	bin := binPath()
	results := []string{}
	if sel.Cursor {
		results = append(results, installCursor(bin))
	}
	if sel.Codex {
		results = append(results, installCodex(bin))
	}
	if sel.OpenCode {
		results = append(results, installOpenCode(bin))
	}
	if sel.Claude {
		results = append(results, installClaude(bin))
	}
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Println("\nReinicie o agent ou recarregue MCP. Teste com: hinfra doctor")
	return nil
}

func ListAgents() {
	fmt.Println("Agents suportados:")
	fmt.Println("  cursor   → ~/.cursor/mcp.json")
	fmt.Println("  claude   → claude mcp add --scope user")
	fmt.Println("  codex    → ~/.codex/config.toml")
	fmt.Println("  opencode → ~/.config/opencode/opencode.json")
}

func RunWizard() error {
	fmt.Println("Selecione agents (ex: cursor,codex ou all):")
	fmt.Print("> ")
	var line string
	if _, err := fmt.Scanln(&line); err != nil {
		return err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "all" {
		return Install(Selection{Cursor: true, Claude: true, Codex: true, OpenCode: true})
	}
	sel := Selection{}
	for _, part := range strings.Split(line, ",") {
		switch strings.TrimSpace(part) {
		case "cursor":
			sel.Cursor = true
		case "claude":
			sel.Claude = true
		case "codex":
			sel.Codex = true
		case "opencode":
			sel.OpenCode = true
		}
	}
	if !sel.Any() {
		return fmt.Errorf("nenhum agent selecionado")
	}
	return Install(sel)
}

func installCursor(bin string) string {
	path := filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json")
	data := map[string]interface{}{}
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &data)
	}
	servers, _ := data["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers[serverName] = map[string]interface{}{
		"command": bin,
		"args":    []string{"mcp"},
	}
	data["mcpServers"] = servers
	if err := writeJSON(path, data); err != nil {
		return fmt.Sprintf("✗ Cursor: %v", err)
	}
	return fmt.Sprintf("✓ Cursor   → %s", path)
}

func installOpenCode(bin string) string {
	path := filepath.Join(os.Getenv("HOME"), ".config", "opencode", "opencode.json")
	data := map[string]interface{}{}
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &data)
	}
	mcp, _ := data["mcp"].(map[string]interface{})
	if mcp == nil {
		mcp = map[string]interface{}{}
	}
	servers, _ := mcp["servers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers[serverName] = map[string]interface{}{
		"type":    "local",
		"command": []string{bin, "mcp"},
	}
	mcp["servers"] = servers
	data["mcp"] = mcp
	if err := writeJSON(path, data); err != nil {
		return fmt.Sprintf("✗ OpenCode: %v", err)
	}
	return fmt.Sprintf("✓ OpenCode → %s", path)
}

func installClaude(bin string) string {
	if _, err := exec.LookPath("claude"); err == nil {
		cmd := exec.Command("claude", "mcp", "add", "--scope", "user", serverName, "--", bin, "mcp")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Sprintf("✗ Claude: %s: %v", out, err)
		}
		return "✓ Claude   → registrado via claude mcp add"
	}
	return fmt.Sprintf("→ Claude   → rode: claude mcp add --scope user %s -- %s mcp", serverName, bin)
}

func writeJSON(path string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
