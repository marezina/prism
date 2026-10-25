// Package autostart starts PRism at login (desktop-integration skill
// §Autostart): Windows HKCU Run key, Linux XDG autostart .desktop file.
// The app always starts with --background from there.
package autostart

import "os"

// Arg is passed when PRism is started at login.
const Arg = "--background"

// Set enables or disables autostart for the running executable.
func Set(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if enabled {
		return enable(exe)
	}
	return disable()
}

// Enabled reports whether autostart is registered.
func Enabled() (bool, error) { return enabled() }
