package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StartChild starts an instance of wfType identified by key under the run
// parentID: a child run, recorded pending for the host's tick to run with
// RunPipeline, which finds it by its key and keeps the parent link. A
// sub-process stage calls it for each child it starts and returns without
// waiting for them: a child that waits on its parent's later stage would
// otherwise deadlock, so waiting is a later stage's Until, reading
// Store.CountDescendants or the host's own condition. A key that already
// exists is returned as it is, whatever its status, so the stage can run
// again after a crash. The parent's id is read back with FindRunByKey.
func (r *Runner) StartChild(ctx context.Context, parentID string, wfType WorkflowType, key string, date BusinessDate) (*RunRecord, error) {
	parent, err := r.store.GetRun(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("finding parent %s: %w", parentID, err)
	}
	if parent == nil {
		return nil, fmt.Errorf("%s %s: no parent run %s", wfType, key, parentID)
	}
	existing, err := r.store.FindRunByKey(ctx, wfType, key)
	if err != nil {
		return nil, fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil {
		return existing, nil
	}
	run := &RunRecord{
		ID:           uuid.New().String(),
		WorkflowType: wfType,
		BusinessDate: date,
		Key:          key,
		ParentID:     parent.ID,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
	}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("creating child run: %w", err)
	}
	return run, nil
}
