package fixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"prism/internal/model"
)

// Context is context.json: the detect context plus a note for hand edits.
type Context struct {
	model.DetectContext
	Note string `json:"note,omitempty"`
}

// MapDir holds the per-scenario mappings, outside the repo because they
// contain original names: <user cache dir>/PRism/fixture-maps.
func MapDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}
	return filepath.Join(dir, "PRism", "fixture-maps"), nil
}

func mapPath(dir, scenario string) string { return filepath.Join(dir, scenario+".json") }

// LoadMapping returns the scenario's mapping, or a new one. Recording a
// scenario as a different user than before is an error: "me" would change.
func LoadMapping(dir, scenario, me string) (*Mapping, error) {
	b, err := os.ReadFile(mapPath(dir, scenario))
	if errors.Is(err, fs.ErrNotExist) {
		return NewMapping(me), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	var m Mapping
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode mapping %s: %w", mapPath(dir, scenario), err)
	}
	if !strings.EqualFold(m.Me, me) {
		return nil, fmt.Errorf("scenario %q was recorded as another user", scenario)
	}
	m.init()
	return &m, nil
}

// SaveMapping writes the mapping readable by the user only.
func SaveMapping(dir, scenario string, m *Mapping) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create mapping dir: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mapPath(dir, scenario), b, 0o600); err != nil {
		return fmt.Errorf("write mapping: %w", err)
	}
	return nil
}

// LoadAllMappings reads every mapping in dir (for sanitize-check).
func LoadAllMappings(dir string) ([]*Mapping, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Mapping
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m Mapping
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("decode %s: %w", e.Name(), err)
		}
		m.init()
		out = append(out, &m)
	}
	return out, nil
}

// WriteJSON writes v indented with a trailing newline.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Finding is one leak found by Check.
type Finding struct {
	File, Match string
}

// Check scans every .json file under root for original names and for
// github.com, which sanitized fixtures never contain.
func Check(root string, originals []string) ([]Finding, error) {
	pats := []*regexp.Regexp{regexp.MustCompile(`github\.com`)}
	for _, o := range originals {
		pats = append(pats, regexp.MustCompile(`(?i)(^|[^a-z0-9_-])`+regexp.QuoteMeta(o)+`($|[^a-z0-9_-])`))
	}
	var findings []Finding
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".json" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, re := range pats {
			if m := re.Find(b); m != nil {
				findings = append(findings, Finding{File: p, Match: strings.Trim(string(m), " \"/:@,.")})
			}
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return findings, err
}
