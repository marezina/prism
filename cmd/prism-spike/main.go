// Command prism-spike holds the Phase 0 platform spikes (docs/PLAN.md) that do
// not need the Wails app: tray badge rendering, keyring and sound.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: prism-spike <badge|keyring|sound> [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "badge":
		err = runBadge(os.Args[2:])
	case "keyring":
		err = runKeyring(os.Args[2:])
	case "sound":
		err = runSound(os.Args[2:])
	default:
		err = fmt.Errorf("unknown spike %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
