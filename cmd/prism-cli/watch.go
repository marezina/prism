package main

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strings"

	"prism/internal/gh"
	"prism/internal/watch"
)

func (c *cli) watch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	dry := fs.Bool("dry", false, "print the watch set and exit")
	owners := fs.String("owners", "", "comma-separated orgs (default: every org without pending SSO)")
	personal := fs.Bool("personal", true, "include your personal repositories")
	drafts := fs.Bool("drafts", true, "include draft PRs")
	combine := fs.Bool("combine", false, "one search for all owners (only if verify-search says qualifiers are OR-ed)")
	snapshots := fs.Bool("snapshots", false, "with --dry: also fetch every snapshot and report the GraphQL cost")
	_ = fs.Parse(args)
	client, id, err := c.client(ctx)
	if err != nil {
		return err
	}
	if !*dry {
		return c.watchLive(ctx, client, id, *owners)
	}
	cfg := watch.Config{Me: id.Login, IncludePersonal: *personal, IncludeDrafts: *drafts, CombineOwners: *combine}
	if cfg.Orgs, err = c.orgList(ctx, client, *owners); err != nil {
		return err
	}
	if cfg.Teams, err = client.Teams(ctx); err != nil {
		return err
	}

	set, err := watch.Build(ctx, client, cfg)
	if err != nil {
		return err
	}
	printWatchSet(set, len(watch.Queries(cfg)))

	if *snapshots {
		res, err := client.Snapshots(ctx, set.IDs())
		if err != nil {
			return err
		}
		fmt.Printf("snapshots: %d fetched, %d missing, %d failed, cost %d points, %d left\n",
			len(res.PRs), len(res.Missing), len(res.Failed), res.Cost, res.Remaining)
		for id, err := range res.Failed {
			fmt.Printf("  failed %s: %v\n", id, err)
		}
	}
	return nil
}

// orgList returns the --owners value or every org that is not SSO-blocked.
func (c *cli) orgList(ctx context.Context, client *gh.Client, flagValue string) ([]string, error) {
	if flagValue != "" {
		var orgs []string
		for _, o := range strings.Split(flagValue, ",") {
			if o = strings.TrimSpace(o); o != "" {
				orgs = append(orgs, o)
			}
		}
		return orgs, nil
	}
	all, err := client.Orgs(ctx)
	if err != nil {
		return nil, err
	}
	var orgs []string
	for _, o := range all {
		if o.SSORequired {
			c.log.Warn("skipping org until SSO is authorized", "org", o.Login, "url", o.SSOURL)
			continue
		}
		orgs = append(orgs, o.Login)
	}
	return orgs, nil
}

func printWatchSet(set watch.Set, queries int) {
	prs := make([]*watch.PR, 0, len(set.PRs))
	for _, pr := range set.PRs {
		prs = append(prs, pr)
	}
	slices.SortFunc(prs, func(a, b *watch.PR) int {
		if c := strings.Compare(a.Repo, b.Repo); c != 0 {
			return c
		}
		return a.Number - b.Number
	})
	for _, pr := range prs {
		roles := make([]string, len(pr.Roles))
		for i, r := range pr.Roles {
			roles[i] = string(r)
		}
		draft := ""
		if pr.IsDraft {
			draft = " [draft]"
		}
		fmt.Printf("%-40s %-28s %s%s\n", fmt.Sprintf("%s#%d", pr.Repo, pr.Number), strings.Join(roles, ","), pr.Title, draft)
	}
	fmt.Printf("\n%d PRs from %d searches, cost %d points\n", len(prs), queries, set.Cost)
	for owner, err := range set.Failed {
		fmt.Printf("  owner %s skipped: %v\n", owner, err)
	}
}
