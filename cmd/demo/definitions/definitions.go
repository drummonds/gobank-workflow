// Package definitions declares the demo bank's workflows and registers
// them with the engine, as a host does: the end of day the engine runs
// itself, and a release pipeline simulated stage by stage. The pages and
// diagrams show these two and nothing else.
package definitions

import wf "git.bytestone.uk/hum3/gobank-workflow"

// ReleaseType is the type of Release.
const ReleaseType wf.WorkflowType = "release"

const srcTaskPlus = "https://git.bytestone.uk/hum3/task-plus/src/branch/main/"

// Release is the release pipeline as task-plus runs it; the demo simulates
// each stage, so the steps link to the real code.
var Release = wf.Definition{
	Type:        ReleaseType,
	Purpose:     wf.Purpose{Name: "release", Description: "Ship a new version of the code."},
	Description: "Ship a new version through the release pipeline. A failed stage stops the release; the next run resumes from that stage.",
	InstanceKey: "version",
	Steps: []wf.Step{
		{Name: "check", Description: "fmt, vet, test", Source: srcTaskPlus + "internal/workflow/workflow.go"},
		{Name: "bump", Description: "choose the next semantic version", Source: srcTaskPlus + "internal/workflow/gather.go"},
		{Name: "changelog", Description: "move [Unreleased] under the new version heading", Source: srcTaskPlus + "internal/changelog/changelog.go"},
		{Name: "tag", Description: "commit and create the annotated git tag", Source: srcTaskPlus + "internal/workflow/execute.go"},
		{Name: "push origin", Description: "push commit and tag to origin", Source: srcTaskPlus + "internal/git/git.go"},
		{Name: "push mirror", Description: "push commit and tag to the GitHub mirror", Source: srcTaskPlus + "internal/git/git.go"},
		{Name: "verify install", Description: "go install the tag from the module proxy", Source: srcTaskPlus + "internal/workflow/execute.go"},
		{Name: "deploy docs", Description: "build and rsync the docs site", Source: srcTaskPlus + "internal/deploy/rsync.go"},
	},
}

func init() {
	wf.Definitions = append(wf.Definitions, wf.DailyUpdate, Release)
}
