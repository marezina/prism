package main

import (
	"context"
	"fmt"
	"strings"
)

func (c *cli) whoami(ctx context.Context) error {
	client, id, err := c.client(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("login:   %s\nname:    %s\nid:      %d\nscopes:  %s\n", id.Login, id.Name, id.ID, strings.Join(id.Scopes, ", "))
	if rl, ok := client.RateLimits().Get("core"); ok {
		fmt.Printf("REST:    %d/%d left, resets %s\n", rl.Remaining, rl.Limit, rl.ResetAt.Format("15:04:05"))
	}
	return nil
}

func (c *cli) orgs(ctx context.Context) error {
	client, id, err := c.client(ctx)
	if err != nil {
		return err
	}
	orgs, err := client.Orgs(ctx)
	if err != nil {
		return err
	}
	teams, err := client.Teams(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Personal repositories: %s\n\nOrganizations:\n", id.Login)
	if len(orgs) == 0 {
		fmt.Println("  (none)")
	}
	for _, o := range orgs {
		if o.SSORequired {
			fmt.Printf("  %-30s SSO authorization required: %s\n", o.Login, o.SSOURL)
			continue
		}
		fmt.Printf("  %s\n", o.Login)
	}
	fmt.Println("\nTeams:")
	if len(teams) == 0 {
		fmt.Println("  (none)")
	}
	for _, t := range teams {
		fmt.Printf("  %s\n", t)
	}
	return nil
}
