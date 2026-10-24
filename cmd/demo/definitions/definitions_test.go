package definitions

import (
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// The registry is the demo's two workflows, each well formed: a type, a
// purpose, a description, an instance key, and every step linked to its code.
func TestTheDemoRegistersItsTwoWorkflows(t *testing.T) {
	if len(wf.Definitions) != 2 || wf.Definitions[0].Type != wf.WorkflowDailyUpdate || wf.Definitions[1].Type != ReleaseType {
		t.Fatalf("registry = %+v", wf.Definitions)
	}
	for _, d := range wf.Definitions {
		if d.Type == "" || d.Description == "" || d.InstanceKey == "" || d.Purpose.Name == "" || d.Purpose.Description == "" {
			t.Fatalf("%s is not fully described: %+v", d.Type, d)
		}
		for _, s := range d.Steps {
			if s.Name == "" || s.Description == "" || !strings.HasPrefix(s.Source, "https://") {
				t.Fatalf("%s step %+v needs a name, a description and a link to its code", d.Type, s)
			}
		}
	}
}
