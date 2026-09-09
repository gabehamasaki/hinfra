package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPullRebaseWithDivergence(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	clone := filepath.Join(root, "clone")
	other := filepath.Join(root, "other")

	gitCmd(bare, "init", "--bare", bare)
	gitCmd(root, "clone", bare, clone)
	gitCmd(clone, "checkout", "-b", "main")
	writeFile(filepath.Join(clone, "README.md"), "init\n")
	gitCmd(clone, "add", ".")
	gitCmd(clone, "commit", "-m", "init")
	gitCmd(clone, "push", "-u", "origin", "main")

	gitCmd(root, "clone", bare, other)
	gitCmd(other, "checkout", "main")
	writeFile(filepath.Join(other, "remote.md"), "remote\n")
	gitCmd(other, "add", ".")
	gitCmd(other, "commit", "-m", "remote")
	gitCmd(other, "push")

	writeFile(filepath.Join(clone, "local.md"), "local\n")
	gitCmd(clone, "add", ".")
	gitCmd(clone, "commit", "-m", "local")

	r := NewRunner(clone)
	if err := r.PullRebase(); err != nil {
		t.Fatalf("pull --rebase failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clone, "remote.md")); err != nil {
		t.Fatal("expected remote file after rebase")
	}
}

func gitCmd(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	if dir != "" && args[0] != "clone" && args[0] != "init" {
		cmd.Dir = dir
	} else if args[0] == "clone" {
		cmd.Dir = filepath.Dir(args[2])
	} else if args[0] == "init" && args[1] == "--bare" {
		cmd.Dir = filepath.Dir(args[2])
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(string(out) + ": " + err.Error())
	}
}

func writeFile(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(content), 0o644)
}
