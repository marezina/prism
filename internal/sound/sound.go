// Package sound plays PRism's embedded notification sounds.
package sound

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"prism/assets"
)

// Player plays one embedded sound by name at volume 0–1
// (desktop-integration skill).
type Player interface {
	Play(name string, volume float64) error
}

// Names lists the embedded sounds ("soft-chime", "alert", ...).
func Names() []string {
	entries, err := assets.Sounds.ReadDir("sounds")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
	}
	sort.Strings(names)
	return names
}

// load decodes every embedded sound.
func load() (map[string]pcm, error) {
	sounds := map[string]pcm{}
	for _, name := range Names() {
		b, err := assets.Sounds.ReadFile("sounds/" + name + ".wav")
		if err != nil {
			return nil, fmt.Errorf("read sound %s: %w", name, err)
		}
		p, err := decodeWAV(b)
		if err != nil {
			return nil, fmt.Errorf("decode sound %s: %w", name, err)
		}
		sounds[name] = p
	}
	return sounds, nil
}
