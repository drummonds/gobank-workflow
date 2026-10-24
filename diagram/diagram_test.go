package diagram

import (
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// shipping is a release pipeline as a host would declare it.
var shipping = wf.Definition{
	Type:        "release",
	Purpose:     wf.Purpose{Name: "release", Description: "Ship a new version of the code."},
	Description: "Ship a new version through the release pipeline.",
	InstanceKey: "version",
	Steps: []wf.Step{
		{Name: "check", Description: "fmt, vet, test", Source: "https://example.org/check.go"},
		{Name: "bump", Description: "choose the next semantic version", Source: "https://example.org/bump.go"},
		{Name: "changelog", Description: "move [Unreleased] under the new version heading", Source: "https://example.org/changelog.go"},
		{Name: "tag", Description: "commit and create the annotated git tag", Source: "https://example.org/tag.go"},
		{Name: "push origin", Description: "push commit and tag to origin", Source: "https://example.org/git.go"},
		{Name: "push mirror", Description: "push commit and tag to the GitHub mirror", Source: "https://example.org/git.go"},
		{Name: "verify install", Description: "go install the tag from the module proxy", Source: "https://example.org/install.go"},
		{Name: "deploy docs", Description: "build and rsync the docs site", Source: "https://example.org/rsync.go"},
	},
}

func release(t *testing.T) wf.Definition { return shipping }

func TestDefinitionDrawsEveryStepInOrder(t *testing.T) {
	src := Definition(release(t), nil)
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
	src := Definition(wf.DailyUpdate, nil)
	if strings.Contains(src, "->") {
		t.Fatalf("single step should have no edge:\n%s", src)
	}
	if !strings.Contains(src, "end_of_day") {
		t.Fatalf("missing the step:\n%s", src)
	}
}

func TestInstanceColoursStepsByState(t *testing.T) {
	def := release(t)
	run := wf.RunRecord{WorkflowType: shipping.Type, Key: "v0.1.3", Status: wf.StatusFailed, Error: "push mirror: boom"}
	steps := []wf.StepResult{
		{StepName: "check", Status: wf.StatusCompleted, DurationNs: 2_000_000},
		{StepName: "bump", Status: wf.StatusCompleted},
		{StepName: "changelog", Status: wf.StatusCompleted},
		{StepName: "tag", Status: wf.StatusCompleted},
		{StepName: "push origin", Status: wf.StatusCompleted},
		{StepName: "push mirror", Status: wf.StatusFailed, Error: "boom"},
		{StepName: "verify install", Status: wf.StatusWaiting},
	}
	src := Instance(def, run, steps, nil)
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

func TestInstanceDrawsTheRevisionItRan(t *testing.T) {
	run := wf.RunRecord{WorkflowType: shipping.Type, Key: "v0.0.9", Status: wf.StatusCompleted,
		Revision: wf.NewRevision([]string{"check", "tag", "publish"})}
	steps := []wf.StepResult{{StepName: "check", Status: wf.StatusCompleted}, {StepName: "tag", Status: wf.StatusCompleted}, {StepName: "publish", Status: wf.StatusCompleted}}
	src := Instance(release(t), run, steps, nil)
	for _, want := range []string{"check -> tag -> publish", `check: "check" {link: "https://`, `publish: "publish"`, "publish.class: completed"} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %q in:\n%s", want, src)
		}
	}
	if strings.Contains(src, "deploy_docs") {
		t.Fatalf("a stage the run's revision did not have should not be drawn:\n%s", src)
	}
}

func TestDefinitionDrawsASubProcessAsACollapsedBoxLinkingToItsChild(t *testing.T) {
	batch := wf.Definition{Type: "batch", InstanceKey: "batch id", Steps: []wf.Step{
		{Name: "describe"}, {Name: "loads", Description: "a load per load of the bed", Child: "load", Many: true}, {Name: "close"}}}
	href := func(t wf.WorkflowType) string { return "/workflows/definitions/" + string(t) }
	src := Definition(batch, href)
	for _, want := range []string{
		`loads: "loads ⊞" {link: "/workflows/definitions/load"}`,
		"loads.style.multiple: true",
		"describe -> loads -> close",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %q in:\n%s", want, src)
		}
	}
	one := wf.Definition{Type: "load", Steps: []wf.Step{{Name: "photos", Child: "photo"}}}
	if src := Definition(one, nil); !strings.Contains(src, `photos: "photos ⊞"`) || strings.Contains(src, "link:") || strings.Contains(src, "multiple") {
		t.Fatalf("one child, no href: a marked box, no link, not stacked:\n%s", src)
	}
	run := wf.RunRecord{WorkflowType: "batch", Key: "B-1", Status: wf.StatusRunning}
	inst := Instance(batch, run, []wf.StepResult{{StepName: "describe", Status: wf.StatusCompleted}, {StepName: "loads", Status: wf.StatusCompleted}}, href)
	if !strings.Contains(inst, `loads: "loads ⊞" {link: "/workflows/definitions/load"}`) || !strings.Contains(inst, "loads.class: completed") {
		t.Fatalf("an instance draws the sub-process the same way:\n%s", inst)
	}
}

// A long step list wraps into rows of four, so a definition stays legible
// across a 960px page; a short one stays a single row.
func TestALongStepListWrapsIntoRows(t *testing.T) {
	long := Definition(shipping, nil)
	if !strings.Contains(long, "grid-columns: 4") || !strings.Contains(long, "check -> bump -> changelog -> tag -> push_origin -> push_mirror -> verify_install -> deploy_docs") {
		t.Fatalf("eight steps should wrap into rows and keep their flow:\n%s", long)
	}
	short := wf.Definition{Type: "promotion", Steps: []wf.Step{{Name: "build"}, {Name: "approve"}, {Name: "promote"}, {Name: "verify"}}}
	if src := Definition(short, nil); strings.Contains(src, "grid") {
		t.Fatalf("four steps fit a row:\n%s", src)
	}
	inst := Instance(shipping, wf.RunRecord{WorkflowType: shipping.Type, Key: "v1"}, nil, nil)
	if !strings.Contains(inst, "grid-columns: 4") {
		t.Fatalf("an instance of a long definition wraps the same way:\n%s", inst)
	}
}
