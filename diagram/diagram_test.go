package diagram

import (
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

func release(t *testing.T) wf.Definition {
	t.Helper()
	for _, d := range wf.Definitions {
		if d.Type == wf.WorkflowRelease {
			return d
		}
	}
	t.Fatal("no release definition")
	return wf.Definition{}
}

func TestDefinitionDrawsEveryStepInOrder(t *testing.T) {
	src := Definition(release(t))
	for _, want := range []string{
		"direction: right",
		`release: "release`,
		`check: "check" {link: "https://`,
		`push_origin: "push origin"`,
		`deploy_docs: "deploy docs"`,
		"check -> bump -> changelog -> tag -> push_origin -> push_mirror -> verify_install -> deploy_docs",
		"one instance per version",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %q in:\n%s", want, src)
		}
	}
}

func TestDefinitionWithOneStepHasNoEdges(t *testing.T) {
	src := Definition(wf.Definitions[0]) // daily_update
	if strings.Contains(src, "->") {
		t.Fatalf("single step should have no edge:\n%s", src)
	}
	if !strings.Contains(src, "end_of_day") {
		t.Fatalf("missing the step:\n%s", src)
	}
}

func TestInstanceColoursStepsByState(t *testing.T) {
	def := release(t)
	run := wf.RunRecord{WorkflowType: wf.WorkflowRelease, Key: "v0.1.3", Status: wf.StatusFailed, Error: "push mirror: boom"}
	steps := []wf.StepResult{
		{StepName: "check", Status: wf.StatusCompleted, DurationNs: 2_000_000},
		{StepName: "bump", Status: wf.StatusCompleted},
		{StepName: "changelog", Status: wf.StatusCompleted},
		{StepName: "tag", Status: wf.StatusCompleted},
		{StepName: "push origin", Status: wf.StatusCompleted},
		{StepName: "push mirror", Status: wf.StatusFailed, Error: "boom"},
		{StepName: "verify install", Status: wf.StatusWaiting},
	}
	src := Instance(def, run, steps)
	if strings.Contains(src, "boom") || strings.Contains(src, "choose the next") {
		t.Fatalf("diagram should carry no detail beyond names and state:\n%s", src)
	}
	for _, want := range []string{
		"v0.1.3",
		"classes:",
		"completed:",
		"failed:",
		"not_run:",
		"check.class: completed",
		"push_mirror.class: failed",
		"waiting:",
		"verify_install.class: waiting",
		"deploy_docs.class: not_run",
		"release v0.1.3 — failed",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %q in:\n%s", want, src)
		}
	}
}

func TestIDsAreSafeD2Identifiers(t *testing.T) {
	for in, want := range map[string]string{"push origin": "push_origin", "end_of_day": "end_of_day", "Verify-Install!": "verify_install"} {
		if got := id(in); got != want {
			t.Fatalf("id(%q) = %q, want %q", in, got, want)
		}
	}
}
