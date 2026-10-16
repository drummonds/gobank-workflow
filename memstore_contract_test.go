package workflow_test

import (
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/storetest"
)

func TestMemStore_KeyedInstances(t *testing.T) { storetest.KeyedInstances(t, wf.NewMemStore()) }
func TestMemStore_RestartAndStepStatus(t *testing.T) {
	storetest.RestartAndStepStatus(t, wf.NewMemStore())
}
func TestMemStore_WaitAndResume(t *testing.T) { storetest.WaitAndResume(t, wf.NewMemStore()) }
func TestMemStore_UndoneStep(t *testing.T)    { storetest.UndoneStep(t, wf.NewMemStore()) }
func TestMemStore_Revisions(t *testing.T)     { storetest.Revisions(t, wf.NewMemStore()) }
