package context

import "testing"

func TestEnvironmentBranchDefault(t *testing.T) {
	e := HinfraEnvironment{}
	if e.EnvironmentBranch() != "main" {
		t.Fatal("expected main")
	}
	e.Branch = "develop"
	if e.EnvironmentBranch() != "develop" {
		t.Fatal("expected develop")
	}
}

func TestEnabledTagTriggersProductionOnly(t *testing.T) {
	cfg := &HinfraConfig{
		App: "demo",
		Environments: map[string]HinfraEnvironment{
			"production": {Enabled: true, TagPrefix: "v"},
		},
	}
	tr := cfg.EnabledTagTriggers()
	if len(tr) != 1 || tr[0] != "v*" {
		t.Fatalf("got %v", tr)
	}
}
