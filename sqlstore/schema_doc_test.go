package sqlstore

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"
)

// The schema page (schema.md) has a column table per table; it must list
// the columns a migrated database has, in order.
func TestSchemaPageListsEveryColumn(t *testing.T) {
	d := newTestDB(t)
	if _, err := New(d); err != nil {
		t.Fatal(err)
	}
	documented := schemaPageColumns(t, "../schema.md")
	for _, table := range []string{"workflow_runs", "workflow_steps", "schema_migrations"} {
		rows, err := d.Query(`SELECT * FROM ` + table + ` LIMIT 0`)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(documented[table], cols) {
			t.Errorf("%s: schema.md lists %v, database has %v", table, documented[table], cols)
		}
	}
}

// schemaPageColumns reads, under each "### `table`" heading, the first
// cell of every table row that names a column in backticks.
func schemaPageColumns(t *testing.T, path string) map[string][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cols := map[string][]string{}
	var table string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "### "); ok {
			table = strings.Trim(name, "`")
			continue
		}
		if table == "" || !strings.HasPrefix(line, "| `") {
			continue
		}
		cell := strings.TrimSpace(strings.Split(line, "|")[1])
		cols[table] = append(cols[table], strings.Trim(cell, "`"))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cols
}
