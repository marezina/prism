package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"prism/internal/gh"
	"prism/internal/poller"
	"prism/internal/store"
)

// openStore opens the app's database (the CLI and the app share it).
func openStore(ctx context.Context) (*store.Store, error) {
	dir, err := store.DataDir()
	if err != nil {
		return nil, err
	}
	return store.Open(ctx, filepath.Join(dir, "prism.db"))
}

// watchLive runs the real poller and prints events until Ctrl+C.
func (c *cli) watchLive(ctx context.Context, client *gh.Client, id gh.Identity, owners string) error {
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	st, err := db.Settings(ctx)
	if err != nil {
		return err
	}
	if owners != "" {
		if st.Owners, err = c.orgList(ctx, client, owners); err != nil {
			return err
		}
		if err := db.SaveSettings(ctx, st); err != nil {
			return err
		}
	}
	if len(st.Owners) == 0 && !st.IncludePersonal {
		return errors.New("no owners selected: pass --owners a,b (saved to settings)")
	}
	if err := db.SetMeta(ctx, store.MetaMeLogin, id.Login); err != nil {
		return err
	}
	teams, err := client.Teams(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Watching %v (personal: %v) every %s. Ctrl+C to stop.\n", st.Owners, st.IncludePersonal, st.PollInterval())
	p := poller.New(poller.Options{
		GitHub: client, Store: db, Me: poller.Identity{Login: id.Login, Teams: teams}, Log: c.log,
		OnEvents: func(fresh []store.Fresh) {
			for _, f := range fresh {
				e := f.Event
				fmt.Printf("%s  %-26s %s#%d  %s %s\n", e.OccurredAt.Local().Format("15:04"), e.Type, e.Repo, e.PRNumber, e.Actor.Login, e.Summary)
				if e.Excerpt != "" {
					fmt.Printf("       %q\n", e.Excerpt)
				}
			}
		},
		OnStatus: func(s poller.Status) {
			if s.Err != nil {
				fmt.Println("sync error:", s.Err)
			}
		},
	})
	err = p.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (c *cli) unread(ctx context.Context) error {
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	list, err := db.Unread(ctx)
	if err != nil {
		return err
	}
	for _, n := range list {
		fmt.Printf("%5d  %s  %-26s %s#%d  %s %s\n", n.ID, n.OccurredAt.Local().Format("Jan 02 15:04"), n.Type,
			n.Repo, n.PRNumber, n.Actor.Login, n.Summary)
	}
	fmt.Printf("%d unread\n", len(list))
	return nil
}

func (c *cli) read(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: prism-cli read <id>... | all")
	}
	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var n int
	if args[0] == "all" {
		n, err = db.MarkAllRead(ctx)
	} else {
		ids := make([]int64, 0, len(args))
		for _, a := range args {
			id, perr := strconv.ParseInt(a, 10, 64)
			if perr != nil {
				return fmt.Errorf("bad id %q", a)
			}
			ids = append(ids, id)
		}
		n, err = db.MarkRead(ctx, ids...)
	}
	if err != nil {
		return err
	}
	fmt.Printf("marked %d as read\n", n)
	return nil
}
