package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"prism/internal/fixture"
	"prism/internal/model"
)

var prRef = regexp.MustCompile(`^([^/\s]+/[^#\s]+)#(\d+)$`)

// record writes one sanitized snapshot for a golden-test scenario
// (event-detector skill, references/fixtures.md).
func (c *cli) record(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("record", flag.ExitOnError)
	pr := flags.String("pr", "", "owner/repo#number")
	out := flags.String("out", "", "testdata/fixtures/<scenario>/<prev|curr>.json")
	_ = flags.Parse(args)

	m := prRef.FindStringSubmatch(*pr)
	if m == nil || *out == "" {
		return errors.New("usage: prism-cli record --pr owner/repo#N --out testdata/fixtures/<scenario>/<step>.json")
	}
	number, _ := strconv.Atoi(m[2])
	scenarioDir := filepath.Dir(*out)
	scenario := filepath.Base(scenarioDir)

	client, id, err := c.client(ctx)
	if err != nil {
		return err
	}
	nodeID, err := client.PRNodeID(ctx, m[1], number)
	if err != nil {
		return err
	}
	res, err := client.Snapshots(ctx, []string{nodeID})
	if err != nil {
		return err
	}
	snap := res.PRs[nodeID]
	if snap == nil {
		return fmt.Errorf("%s: no snapshot (%v)", *pr, res.Failed[nodeID])
	}

	mapDir, err := fixture.MapDir()
	if err != nil {
		return err
	}
	mapping, err := fixture.LoadMapping(mapDir, scenario, id.Login)
	if err != nil {
		return err
	}
	clean := mapping.Snapshot(snap)

	ctxPath := filepath.Join(scenarioDir, "context.json")
	if _, err := os.Stat(ctxPath); errors.Is(err, fs.ErrNotExist) {
		teams, err := client.Teams(ctx)
		if err != nil {
			return err
		}
		fc := fixture.Context{DetectContext: model.DetectContext{
			Me: "me", MyTeams: mapping.TeamNames(teams), IgnoreBots: true,
		}}
		if err := fixture.WriteJSON(ctxPath, fc); err != nil {
			return err
		}
		fmt.Println("wrote", ctxPath)
	}

	if err := fixture.WriteJSON(*out, clean); err != nil {
		return err
	}
	if err := fixture.SaveMapping(mapDir, scenario, mapping); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d reviews, %d comments, %d threads); mapping kept outside the repo in %s\n",
		*out, len(clean.Reviews), len(clean.Comments), len(clean.Threads), mapDir)
	return c.sanitizeCheck([]string{"--dir", scenarioDir})
}

// sanitizeCheck fails if any original name from any mapping is in a fixture.
func (c *cli) sanitizeCheck(args []string) error {
	flags := flag.NewFlagSet("sanitize-check", flag.ExitOnError)
	dir := flags.String("dir", "testdata/fixtures", "fixtures root")
	_ = flags.Parse(args)

	mapDir, err := fixture.MapDir()
	if err != nil {
		return err
	}
	maps, err := fixture.LoadAllMappings(mapDir)
	if err != nil {
		return err
	}
	var originals []string
	for _, m := range maps {
		originals = append(originals, m.Originals()...)
	}
	findings, err := fixture.Check(*dir, originals)
	if err != nil {
		return err
	}
	for _, f := range findings {
		fmt.Printf("LEAK %s: %q\n", f.File, f.Match)
	}
	if len(findings) > 0 {
		return fmt.Errorf("sanitize-check: %d leaks", len(findings))
	}
	fmt.Printf("sanitize-check: %s clean (%d names from %d mappings)\n", *dir, len(originals), len(maps))
	return nil
}
