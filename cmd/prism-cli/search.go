package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"maps"
	"slices"
	"strings"

	"prism/internal/gh"
)

func (c *cli) search(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New(`usage: prism-cli search "<query>"`)
	}
	client, _, err := c.client(ctx)
	if err != nil {
		return err
	}
	res, err := client.SearchPRs(ctx, args[0])
	if err != nil {
		return err
	}
	for _, h := range res.Hits {
		draft := ""
		if h.IsDraft {
			draft = " [draft]"
		}
		fmt.Printf("%s#%d%s  %s\n", h.Repo, h.Number, draft, h.Title)
	}
	fmt.Printf("\n%d PRs (issueCount %d), cost %d, %d points left\n", len(res.Hits), res.IssueCount, res.Cost, res.Remaining)
	return nil
}

// verifySearch answers ADR 0005: are several org:/user: qualifiers in one
// search OR-ed, and does @me work in GraphQL search? It compares the combined
// query with one query per owner, and @me with the literal login.
func (c *cli) verifySearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify-search", flag.ExitOnError)
	ownersFlag := fs.String("owners", "", "comma-separated orgs (default: all accessible orgs); your user is always added")
	_ = fs.Parse(args)

	client, id, err := c.client(ctx)
	if err != nil {
		return err
	}
	var orgs []string
	if *ownersFlag != "" {
		orgs = strings.Split(*ownersFlag, ",")
	} else {
		all, err := client.Orgs(ctx)
		if err != nil {
			return err
		}
		for _, o := range all {
			if !o.SSORequired {
				orgs = append(orgs, o.Login)
			}
		}
	}
	quals := []string{"user:" + id.Login}
	for _, o := range orgs {
		quals = append(quals, "org:"+strings.TrimSpace(o))
	}

	cost := 0
	run := func(q string) (map[string]gh.SearchHit, error) {
		res, err := client.SearchPRs(ctx, q)
		if err != nil {
			return nil, err
		}
		cost += res.Cost
		set := map[string]gh.SearchHit{}
		for _, h := range res.Hits {
			set[h.ID] = h
		}
		return set, nil
	}

	for _, base := range []string{"is:pr author:@me", "is:pr involves:@me"} {
		fmt.Printf("== %s\n", base)
		union := map[string]gh.SearchHit{}
		var perOwner []map[string]gh.SearchHit
		for _, q := range quals {
			set, err := run(base + " " + q)
			if err != nil {
				return err
			}
			fmt.Printf("  %-40s %4d\n", q, len(set))
			maps.Copy(union, set)
			perOwner = append(perOwner, set)
		}
		combined, err := run(base + " " + strings.Join(quals, " "))
		if err != nil {
			return err
		}
		fmt.Printf("  %-40s %4d   (union of the above: %d)\n", "all qualifiers together", len(combined), len(union))
		fmt.Printf("  → %s\n", verdict(combined, union, perOwner))
	}

	fmt.Printf("== @me vs literal login (is:pr author:…)\n")
	me, err := run("is:pr author:@me")
	if err != nil {
		return err
	}
	literal, err := run("is:pr author:" + id.Login)
	if err != nil {
		return err
	}
	fmt.Printf("  author:@me %d, author:%s %d → ", len(me), id.Login, len(literal))
	switch {
	case len(literal) == 0:
		fmt.Println("inconclusive (you have no PRs)")
	case sameKeys(me, literal):
		fmt.Println("@me works in GraphQL search")
	default:
		fmt.Println("@me does NOT match the literal login — use the login")
	}
	fmt.Printf("\nGraphQL cost of this check: %d points\n", cost)
	return nil
}

func verdict(combined, union map[string]gh.SearchHit, perOwner []map[string]gh.SearchHit) string {
	nonEmpty := 0
	for _, s := range perOwner {
		if len(s) > 0 {
			nonEmpty++
		}
	}
	switch {
	case nonEmpty < 2:
		return "inconclusive: need hits in at least two owners"
	case sameKeys(combined, union):
		return "qualifiers are OR-ed: one search for all owners is enough"
	case len(combined) < len(union):
		return "qualifiers are NOT OR-ed (combined finds less): run one search per owner"
	default:
		return fmt.Sprintf("unexpected: combined has %d extra results", len(combined)-len(union))
	}
}

func sameKeys(a, b map[string]gh.SearchHit) bool {
	return slices.Equal(slices.Sorted(maps.Keys(a)), slices.Sorted(maps.Keys(b)))
}
