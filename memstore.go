package workflow

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemStore is an in-memory Store for testing and WASM demos.
type MemStore struct {
	mu    sync.Mutex
	runs  []RunRecord
	steps []StepResult
}

// NewMemStore creates an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{}
}

var _ Store = (*MemStore)(nil)

func (m *MemStore) CreateRun(ctx context.Context, run *RunRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// One instance per (type, date): a completed one is final, an
	// unfinished one is replaced by this new attempt along with its steps.
	kept := m.runs[:0]
	for _, r := range m.runs {
		if r.WorkflowType == run.WorkflowType && r.BusinessDate.Equal(run.BusinessDate) && r.Key == run.Key {
			if r.Status == StatusCompleted {
				return ErrAlreadyCompleted
			}
			m.dropSteps(r.ID)
			continue
		}
		kept = append(kept, r)
	}
	m.runs = kept
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	stored := *run
	stored.Revision.Steps = slices.Clone(run.Revision.Steps)
	m.runs = append(m.runs, stored)
	return nil
}

func (m *MemStore) dropSteps(runID string) {
	kept := m.steps[:0]
	for _, st := range m.steps {
		if st.RunID != runID {
			kept = append(kept, st)
		}
	}
	m.steps = kept
}

func (m *MemStore) GetRun(ctx context.Context, id string) (*RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id {
			r := m.runs[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (m *MemStore) FindRun(ctx context.Context, wfType WorkflowType, date BusinessDate) (*RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].WorkflowType == wfType && m.runs[i].BusinessDate.Equal(date) && m.runs[i].Key == "" {
			r := m.runs[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (m *MemStore) FindRunByKey(ctx context.Context, wfType WorkflowType, key string) (*RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].WorkflowType == wfType && m.runs[i].Key == key {
			r := m.runs[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (m *MemStore) RestartRun(ctx context.Context, id string, rev Revision) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id {
			m.runs[i].Revision = Revision{ID: rev.ID, Steps: slices.Clone(rev.Steps)}
			m.runs[i].Status = StatusRunning
			m.runs[i].StartedAt = time.Now()
			m.runs[i].CompletedAt = time.Time{}
			m.runs[i].Error = ""
			return nil
		}
	}
	return nil
}

func (m *MemStore) CancelRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id {
			m.runs[i].Status = StatusCancelled
			m.runs[i].CompletedAt = time.Now()
			return nil
		}
	}
	return nil
}

func (m *MemStore) WaitRun(ctx context.Context, id string) error {
	return m.setStatus(id, StatusWaiting)
}

func (m *MemStore) ResumeRun(ctx context.Context, id string) error {
	return m.setStatus(id, StatusRunning)
}

// setStatus moves a run between the in-flight statuses without touching
// its timestamps: the attempt is the same one.
func (m *MemStore) setStatus(id string, status RunStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == id && m.runs[i].Status != StatusCancelled {
			m.runs[i].Status = status
			m.runs[i].Error = ""
			return nil
		}
	}
	return nil
}

func (m *MemStore) DeleteSteps(ctx context.Context, runID string, statuses ...RunStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	kept := m.steps[:0]
	for _, st := range m.steps {
		if st.RunID == runID && (len(statuses) == 0 || slices.Contains(statuses, st.Status)) {
			continue
		}
		kept = append(kept, st)
	}
	m.steps = kept
	return nil
}

func (m *MemStore) DeleteStep(ctx context.Context, runID, stepName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	kept := m.steps[:0]
	for _, st := range m.steps {
		if st.RunID == runID && st.StepName == stepName {
			continue
		}
		kept = append(kept, st)
	}
	m.steps = kept
	return nil
}

func (m *MemStore) CompleteRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id && m.runs[i].Status != StatusCancelled {
			m.runs[i].Status = StatusCompleted
			m.runs[i].CompletedAt = time.Now()
			return nil
		}
	}
	return nil
}

func (m *MemStore) FailRun(ctx context.Context, id string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id && m.runs[i].Status != StatusCancelled {
			m.runs[i].Status = StatusFailed
			m.runs[i].CompletedAt = time.Now()
			m.runs[i].Error = errMsg
			return nil
		}
	}
	return nil
}

func (m *MemStore) RecordStep(ctx context.Context, step *StepResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if step.CreatedAt.IsZero() {
		step.CreatedAt = time.Now()
	}
	m.steps = append(m.steps, *step)
	return nil
}

func (m *MemStore) ListSteps(ctx context.Context, runID string) ([]StepResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []StepResult
	for _, s := range m.steps {
		if s.RunID == runID {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (m *MemStore) ListRuns(ctx context.Context, f RunFilter) ([]RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var sorted []RunRecord
	for _, r := range m.runs {
		if f.matches(r) {
			sorted = append(sorted, r)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if f.ActiveFirst {
			if hi, hj := sorted[i].Status.Active(), sorted[j].Status.Active(); hi != hj {
				return hi
			}
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	sorted = sorted[min(f.Offset, len(sorted)):]
	if f.Limit > 0 && len(sorted) > f.Limit {
		sorted = sorted[:f.Limit]
	}
	return sorted, nil
}

func (f RunFilter) matches(r RunRecord) bool {
	return (len(f.Types) == 0 || slices.Contains(f.Types, r.WorkflowType)) &&
		(len(f.Statuses) == 0 || slices.Contains(f.Statuses, r.Status)) &&
		!r.CreatedAt.Before(f.Since.Truncate(time.Second)) &&
		strings.Contains(strings.ToLower(r.Key), strings.ToLower(f.KeyContains)) &&
		(f.Revision == "" || r.Revision.ID == f.Revision) &&
		(f.ParentID == "" || r.ParentID == f.ParentID)
}

func (m *MemStore) CountDescendants(ctx context.Context, id string) (RunCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counts := RunCounts{}
	parents := []string{id}
	for len(parents) > 0 {
		var next []string
		for _, r := range m.runs {
			if r.ParentID != "" && slices.Contains(parents, r.ParentID) {
				counts[r.Status]++
				next = append(next, r.ID)
			}
		}
		parents = next
	}
	return counts, nil
}

func (m *MemStore) LastCompletedDate(ctx context.Context, wfType WorkflowType) (BusinessDate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var latest BusinessDate
	for _, r := range m.runs {
		if r.WorkflowType == wfType && r.Status == StatusCompleted {
			if latest.IsZero() || r.BusinessDate.After(latest) {
				latest = r.BusinessDate
			}
		}
	}
	return latest, nil
}

func (m *MemStore) CountRuns(ctx context.Context) (map[WorkflowType]RunCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counts := map[WorkflowType]RunCounts{}
	for _, r := range m.runs {
		if counts[r.WorkflowType] == nil {
			counts[r.WorkflowType] = RunCounts{}
		}
		counts[r.WorkflowType][r.Status]++
	}
	return counts, nil
}
