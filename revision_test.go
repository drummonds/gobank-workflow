package workflow

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// --- Purpose: what a workflow achieves -------------------------------------

func TestEveryDefinitionHasAPurpose(t *testing.T) {
	for _, d := range []Definition{DailyUpdate} {
		if d.Purpose.Name == "" || d.Purpose.Description == "" {
			t.Fatalf("%s needs a purpose with a name and a description, got %+v", d.Type, d.Purpose)
		}
	}
}

// --- Revision: the step list as the code had it ----------------------------

func TestRevisionIdentifiesTheStepList(t *testing.T) {
	a := NewRevision([]string{"create", "serve", "remove"})
	if len(a.ID) != 16 {
		t.Fatalf("revision id %q should be 16 hex characters", a.ID)
	}
	if !slices.Equal(a.Steps, []string{"create", "serve", "remove"}) {
		t.Fatalf("revision should keep its step names, got %v", a.Steps)
	}
	if b := NewRevision([]string{"create", "serve", "remove"}); b.ID != a.ID {
		t.Fatalf("the same step list is the same revision: %s vs %s", a.ID, b.ID)
	}
	for _, other := range [][]string{
		{"serve", "create", "remove"},
		{"create", "serve"},
		{"create serve", "remove"},
	} {
		if NewRevision(other).ID == a.ID {
			t.Fatalf("%v should be a different revision from %v", other, a.Steps)
		}
	}
}

func TestADefinitionsRevisionIsItsStepNames(t *testing.T) {
	for _, d := range []Definition{DailyUpdate, {Type: "ship", Steps: []Step{{Name: "build"}, {Name: "tag"}}}} {
		var names []string
		for _, s := range d.Steps {
			names = append(names, s.Name)
		}
		if got, want := d.Revision(), NewRevision(names); got.ID != want.ID || !slices.Equal(got.Steps, want.Steps) {
			t.Fatalf("%s revision = %+v, want %+v", d.Type, got, want)
		}
	}
}

// --- The runner records the revision it ran --------------------------------

func TestRunPipelineRecordsTheRevisionOfItsStages(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	stages := []Stage{{Name: "check"}, {Name: "tag"}}
	rec, err := NewRunner(store, nil).RunPipeline(ctx, release, "v1.0.0", NewBusinessDate(2026, time.October, 16), stages)
	if err != nil {
		t.Fatal(err)
	}
	if want := NewRevision([]string{"check", "tag"}); rec.Revision.ID != want.ID || !slices.Equal(rec.Revision.Steps, want.Steps) {
		t.Fatalf("run revision = %+v, want %+v", rec.Revision, want)
	}
}

func TestARetryUnderChangedCodeCarriesTheNewRevision(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.October, 16)
	fail := func(context.Context) error { return errors.New("boom") }
	if _, err := r.RunPipeline(ctx, release, "v1.0.1", d, []Stage{{Name: "check"}, {Name: "tag", Run: fail}}); err == nil {
		t.Fatal("first attempt should fail")
	}
	changed := []Stage{{Name: "check"}, {Name: "changelog"}, {Name: "tag"}}
	rec, err := r.RunPipeline(ctx, release, "v1.0.1", d, changed)
	if err != nil {
		t.Fatal(err)
	}
	if want := NewRevision([]string{"check", "changelog", "tag"}); rec.Revision.ID != want.ID {
		t.Fatalf("retry revision = %+v, want the new attempt's %+v", rec.Revision, want)
	}
}

func TestRunDailyRecordsTheEndOfDayRevision(t *testing.T) {
	ctx := context.Background()
	rec, err := NewRunner(NewMemStore(), &failOnceProcessor{calls: 1}).RunDaily(ctx, NewBusinessDate(2026, time.October, 16))
	if err != nil {
		t.Fatal(err)
	}
	if want := NewRevision([]string{"end_of_day"}); rec.Revision.ID != want.ID {
		t.Fatalf("daily revision = %+v, want %+v", rec.Revision, want)
	}
}

func TestStepsForARevisionAreDescribedByTheDefinitionWhereTheyStillExist(t *testing.T) {
	d := Definition{Steps: []Step{{"check", "fmt, vet, test", "https://check"}, {"tag", "annotated tag", "https://tag"}}}
	got := d.StepsFor(NewRevision([]string{"check", "publish"}))
	want := []Step{{"check", "fmt, vet, test", "https://check"}, {Name: "publish"}}
	if !slices.Equal(got, want) {
		t.Fatalf("StepsFor = %+v, want %+v", got, want)
	}
	if got := d.StepsFor(Revision{}); !slices.Equal(got, d.Steps) {
		t.Fatalf("a run recorded before revisions should get the definition's steps, got %+v", got)
	}
}
