package main

import (
	"flag"
	"fmt"
	"time"

	"prism/internal/sound"
)

// runSound plays every embedded sound (or one, with -name) at -volume.
func runSound(args []string) error {
	fs := flag.NewFlagSet("sound", flag.ExitOnError)
	name := fs.String("name", "", "play only this sound")
	volume := fs.Float64("volume", 0.7, "volume 0–1")
	_ = fs.Parse(args)

	player, err := sound.NewOtoPlayer()
	if err != nil {
		return err
	}
	names := sound.Names()
	if *name != "" {
		names = []string{*name}
	}
	for _, n := range names {
		fmt.Printf("playing %s at %.1f\n", n, *volume)
		if err := player.Play(n, *volume); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return nil
}
