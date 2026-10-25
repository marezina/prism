package detect

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"prism/internal/model"
)

// -update writes expected.json only for scenarios that have none; review the
// file by hand before committing (event-detector skill, step 5).
var update = flag.Bool("update", false, "create missing expected.json files")

const fixturesDir = "../../testdata/fixtures"

type fixtureContext struct {
	model.DetectContext
	Note string `json:"note,omitempty"`
}

func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(fixturesDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no fixtures found in " + fixturesDir)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			var c fixtureContext
			readJSON(t, filepath.Join(dir, "context.json"), &c)
			var prev, curr *model.PRSnapshot
			readJSON(t, filepath.Join(dir, "prev.json"), &prev)
			readJSON(t, filepath.Join(dir, "curr.json"), &curr)

			got := Diff(prev, curr, c.DetectContext)

			expPath := filepath.Join(dir, "expected.json")
			if _, err := os.Stat(expPath); errors.Is(err, fs.ErrNotExist) && *update {
				writeJSON(t, expPath, got)
				t.Logf("wrote %s — review it by hand", expPath)
			}
			var want []model.Event
			readJSON(t, expPath, &want)
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("%s\n(-want +got):\n%s", c.Note, diff)
			}

			// Deterministic: the same pair gives the same output.
			if again := Diff(prev, curr, c.DetectContext); !cmp.Equal(got, again, cmpopts.EquateEmpty()) {
				t.Errorf("second run differs:\n%s", cmp.Diff(got, again))
			}
			// Nothing is new when nothing changed (on a PR already being watched).
			seen := c.DetectContext
			seen.FirstSeen = false
			if same := Diff(curr, curr, seen); len(same) != 0 {
				t.Errorf("Diff(curr, curr) = %v, want none", same)
			}
		})
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && filepath.Base(path) == "prev.json" {
		return // a missing prev means "first seen"
	}
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
