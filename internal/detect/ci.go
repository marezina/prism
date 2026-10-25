package detect

import (
	"fmt"
	"strings"
	"time"

	"prism/internal/model"
)

// maxCheckNames is how many failing checks the ci.failed summary names.
const maxCheckNames = 3

// detectCI: the head commit's rollup of my PR turns FAILURE/ERROR (ci.failed)
// or SUCCESS after pending or failure (ci.passed); a new head starts fresh
// (SPEC §3.2). No CI (nil rollup) is not a failure.
func detectCI(d *diff) []candidate {
	if !d.mine || d.curr.CI == nil {
		return nil
	}
	state := d.curr.CI.State
	before := model.CheckPending // new head, or CI appearing: treat as pending
	if d.prev.CI != nil && d.prev.HeadOID == d.curr.HeadOID {
		before = d.prev.CI.State
	}
	prefix := "ci:" + d.curr.ID + ":" + d.curr.HeadOID

	switch {
	case failed(state) && !failed(before):
		var names []string
		var at time.Time
		url := d.curr.URL + "/checks"
		for _, c := range d.curr.CI.Checks {
			if !failed(c.State) {
				continue
			}
			if len(names) == 0 && c.URL != "" {
				url = c.URL
			}
			names = append(names, c.Name)
			at = later(at, c.At)
		}
		summary := "CI failed"
		if len(names) > 0 {
			summary += ": " + strings.Join(names[:min(len(names), maxCheckNames)], ", ")
			if extra := len(names) - maxCheckNames; extra > 0 {
				summary += fmt.Sprintf(" +%d more", extra)
			}
		}
		key := prefix + ":fail"
		return []candidate{{source: key, event: d.event(model.CIFailed, key, model.Actor{}, summary, "", url, d.orUpdated(at))}}

	case state == model.CheckSuccess && before != model.CheckSuccess:
		var at time.Time
		for _, c := range d.curr.CI.Checks {
			at = later(at, c.At)
		}
		key := prefix + ":ok"
		return []candidate{{source: key, event: d.event(model.CIPassed, key, model.Actor{}, "CI passed", "", d.curr.URL+"/checks", d.orUpdated(at))}}
	}
	return nil
}

func failed(s model.CheckState) bool { return s == model.CheckFailure || s == model.CheckError }

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// orUpdated falls back to the PR's updatedAt when GitHub gave no time.
func (d *diff) orUpdated(t time.Time) time.Time {
	if t.IsZero() {
		return d.curr.UpdatedAt
	}
	return t
}
