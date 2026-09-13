package config

import "testing"

func TestResolveArgocdRootApp(t *testing.T) {
	infra := "/infra"
	workloads := "/workloads"

	if got := ResolveArgocdRootApp("custom-root", infra, workloads); got != "custom-root" {
		t.Fatalf("explicit: got %q", got)
	}
	if got := ResolveArgocdRootApp("", infra, workloads); got != "workloads-root" {
		t.Fatalf("separate workloads: got %q", got)
	}
	if got := ResolveArgocdRootApp("", infra, infra); got != "root-app" {
		t.Fatalf("single repo: got %q", got)
	}
}
