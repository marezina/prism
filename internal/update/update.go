// Package update checks the internal release repo for a newer PRism
// (release skill §Update check). It only reads; installing is manual for now.
package update

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"prism/internal/gh"
)

// Repo is the "owner/name" of the release repository, set at build time:
// -ldflags "-X prism/internal/update.Repo=acme/prism". Empty disables checks.
var Repo = ""

// Release is the newest published release.
type Release struct {
	Tag string // e.g. v0.2.0
	URL string // release page
}

// ErrDisabled means no release repo was configured or this is a dev build.
var ErrDisabled = errors.New("update check disabled")

// Newer reports whether tag is a newer version than current. Pre-releases
// count (beta testers want the next beta); invalid versions never are newer.
func Newer(current, tag string) bool {
	c, t := canonical(current), canonical(tag)
	return semver.IsValid(c) && semver.IsValid(t) && semver.Compare(t, c) > 0
}

func canonical(v string) string {
	if v != "" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Latest returns the newest release, including pre-releases, using the
// user's token (the internal repo is private).
func Latest(ctx context.Context, c *gh.Client, repo string) (Release, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return Release{}, ErrDisabled
	}
	releases, _, err := c.REST.Repositories.ListReleases(ctx, owner, name, nil)
	if err != nil {
		return Release{}, fmt.Errorf("list releases of %s: %w", repo, err)
	}
	var best Release
	for _, r := range releases {
		if r.GetDraft() {
			continue
		}
		if best.Tag == "" || Newer(best.Tag, r.GetTagName()) {
			best = Release{Tag: r.GetTagName(), URL: r.GetHTMLURL()}
		}
	}
	if best.Tag == "" {
		return Release{}, errors.New("no releases published")
	}
	return best, nil
}
