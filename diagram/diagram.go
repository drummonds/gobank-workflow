// Package diagram renders workflow definitions and instances as d2 source,
// so the pictures in the docs and the app are generated from the same
// registry the engine runs, and stay right as workflows grow. The diagrams
// show how the steps flow and nothing else; descriptions, durations and
// errors belong in the tables beside them.
package diagram

import (
	"fmt"
	"strings"
	"unicode"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// Definition draws the steps of a definition in order.
func Definition(d wf.Definition) string {
	var b strings.Builder
	b.WriteString("direction: right\n\n")
	fmt.Fprintf(&b, "%s: %q {\n", id(string(d.Type)), fmt.Sprintf("%s — one instance per %s", d.Type, d.InstanceKey))
	writeSteps(&b, d.Steps)
	b.WriteString("}\n")
	return b.String()
}

// Instance draws the steps the instance ran, from its revision, each in
// the state the instance left it: completed, failed, or not run.
func Instance(d wf.Definition, run wf.RunRecord, steps []wf.StepResult) string {
	byName := map[string]wf.StepResult{}
	for _, s := range steps {
		byName[s.StepName] = s
	}
	var b strings.Builder
	b.WriteString("direction: right\n\n")
	b.WriteString("classes: {\n")
	b.WriteString("  completed: {style.fill: \"#d1fae5\"; style.stroke: \"#059669\"}\n")
	b.WriteString("  failed: {style.fill: \"#fee2e2\"; style.stroke: \"#dc2626\"; style.bold: true}\n")
	b.WriteString("  waiting: {style.fill: \"#fef3c7\"; style.stroke: \"#d97706\"; style.bold: true}\n")
	b.WriteString("  not_run: {style.fill: \"#f3f4f6\"; style.stroke: \"#9ca3af\"; style.stroke-dash: 3}\n")
	b.WriteString("}\n\n")
	name := run.Key
	if name == "" {
		name = run.BusinessDate.String()
	}
	fmt.Fprintf(&b, "%s: %q {\n", id(string(d.Type)), fmt.Sprintf("%s %s — %s", d.Type, name, run.Status))
	ran := d.StepsFor(run.Revision)
	writeSteps(&b, ran)
	for _, st := range ran {
		class := "not_run"
		if s, ok := byName[st.Name]; ok {
			switch s.Status {
			case wf.StatusCompleted:
				class = "completed"
			case wf.StatusFailed:
				class = "failed"
			case wf.StatusWaiting:
				class = "waiting"
			}
		}
		fmt.Fprintf(&b, "  %s.class: %s\n", id(st.Name), class)
	}
	b.WriteString("}\n")
	return b.String()
}

func writeSteps(b *strings.Builder, steps []wf.Step) {
	var ids []string
	for _, st := range steps {
		if st.Source != "" {
			// A clickable node: the step's code.
			fmt.Fprintf(b, "  %s: %q {link: %q}\n", id(st.Name), st.Name, st.Source)
		} else {
			fmt.Fprintf(b, "  %s: %q\n", id(st.Name), st.Name)
		}
		ids = append(ids, id(st.Name))
	}
	if len(ids) > 1 {
		fmt.Fprintf(b, "  %s\n", strings.Join(ids, " -> "))
	}
}

// id turns a step or definition name into a d2 identifier.
func id(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
