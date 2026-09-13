package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTailnetAPIAddrFromKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kc.yaml")
	content := `apiVersion: v1
kind: Config
current-context: c
contexts:
- name: c
  context:
    cluster: cl
    user: u
clusters:
- name: cl
  cluster:
    server: https://100.86.241.1:6443
users:
- name: u
  user: {}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	addr, err := TailnetAPIAddr(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if addr != "100.86.241.1:6443" {
		t.Fatalf("got %q", addr)
	}
}
