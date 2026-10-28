// Package definitions declares the demo bank's workflows and registers
// them with the engine, as a host does: the end of day the engine runs
// itself, a release pipeline simulated stage by stage, and a payment batch
// whose payments are instances of their own. The pages and diagrams show
// these four and nothing else.
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

// BatchType is the type of PaymentBatch; PaymentType the type of Payment.
const (
	BatchType   wf.WorkflowType = "payment_batch"
	PaymentType wf.WorkflowType = "payment"
)

const srcDemo = "https://git.bytestone.uk/hum3/gobank-workflow/src/branch/main/cmd/demo/payments.go"

// PaymentBatch is a file of payments paid as one instance: a master whose
// details are the payments, each a child instance of Payment. The batch
// starts them and does not wait; settling does.
var PaymentBatch = wf.Definition{
	Type:        BatchType,
	Purpose:     wf.Purpose{Name: "payments", Description: "Pay the customers' instructions out of their accounts."},
	Description: "Pay a file of payments. Each payment is an instance of its own under the batch; the batch settles once every payment has finished, and fails if any of them failed.",
	InstanceKey: "batch reference",
	Steps: []wf.Step{
		{Name: "receive", Description: "take the file in: count the payments and total them", Source: srcDemo},
		{Name: "start payments", Description: "start one payment instance per payment in the file", Source: srcDemo, Child: PaymentType, Many: true},
		{Name: "settle", Description: "wait until no payment is still active; fail the batch if any payment failed", Source: srcDemo},
	},
}

// Payment is one payment of a batch: a detail of its master, keyed by the
// batch reference and its position in the file.
var Payment = wf.Definition{
	Type:        PaymentType,
	Purpose:     PaymentBatch.Purpose,
	Description: "Pay one payment of a batch out of the paying account.",
	InstanceKey: "batch reference/position",
	Steps: []wf.Step{
		{Name: "check funds", Description: "the paying account covers the amount", Source: srcDemo},
		{Name: "post", Description: "debit the paying account", Source: srcDemo},
	},
}

func init() {
	wf.Definitions = append(wf.Definitions, wf.DailyUpdate, Release, PaymentBatch, Payment)
}
