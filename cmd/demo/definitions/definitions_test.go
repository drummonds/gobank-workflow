package definitions

import (
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// The registry is the demo's four workflows, each well formed: a type, a
// purpose, a description, an instance key, and every step linked to its code.
func TestTheDemoRegistersItsFourWorkflows(t *testing.T) {
	want := []wf.WorkflowType{wf.WorkflowDailyUpdate, ReleaseType, BatchType, PaymentType}
	if len(wf.Definitions) != len(want) {
		t.Fatalf("registry = %+v", wf.Definitions)
	}
	for i, d := range wf.Definitions {
		if d.Type != want[i] {
			t.Fatalf("registry[%d] = %s, want %s", i, d.Type, want[i])
		}
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

// A batch's sub-process step starts its payments all at once, each an
// instance of the payment definition given its place in the file; a later
// step of the batch joins them.
func TestABatchStartsItsPaymentsAllAtOnceAndALaterStepJoinsThem(t *testing.T) {
	var start, joins int
	for i, s := range PaymentBatch.Steps {
		if s.Child == PaymentType {
			if s.Mode != wf.AllAtOnce {
				t.Fatalf("%s starts payments %q, want all at once", s.Name, s.Mode)
			}
			if s.Given == "" {
				t.Fatalf("%s says nothing of what a payment is given", s.Name)
			}
			start = i
		}
		if s.Joins != "" {
			if s.Joins != "start payments" {
				t.Fatalf("%s joins %q, want the step that starts the payments", s.Name, s.Joins)
			}
			joins = i
		}
	}
	if start == 0 || start == len(PaymentBatch.Steps)-1 {
		t.Fatalf("payments should be started after receiving and before settling: %+v", PaymentBatch.Steps)
	}
	if joins <= start {
		t.Fatalf("the step that joins the payments should come after the one that starts them: %+v", PaymentBatch.Steps)
	}
	for _, s := range Payment.Steps {
		if s.Child != "" {
			t.Fatalf("a payment has no children: %+v", s)
		}
	}
}
