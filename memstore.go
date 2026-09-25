package workflow

import (
	"context"
	"sort"
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

	for _, r := range m.runs {
		if r.WorkflowType == run.WorkflowType && r.BusinessDate.Equal(run.BusinessDate) {
			if r.Status == StatusCompleted {
				return ErrAlreadyCompleted
			}
		}
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	m.runs = append(m.runs, *run)
	return nil
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
		if m.runs[i].WorkflowType == wfType && m.runs[i].BusinessDate.Equal(date) {
			r := m.runs[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (m *MemStore) CompleteRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.runs {
		if m.runs[i].ID == id {
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
		if m.runs[i].ID == id {
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

func (m *MemStore) ListRuns(ctx context.Context, limit int) ([]RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sorted := make([]RunRecord, len(m.runs))
	copy(sorted, m.runs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted, nil
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
