package autostart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func desktopFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "prism.desktop"), nil
}

// entry is the XDG autostart file; exec is quoted per the Desktop Entry spec.
func entry(exe string) string {
	quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(exe) + `"`
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=PRism
Comment=GitHub pull-request notifications
Exec=%s %s
Icon=prism
Terminal=false
X-GNOME-Autostart-enabled=true
`, quoted, Arg)
}

func enable(exe string) error {
	p, err := desktopFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.WriteFile(p, []byte(entry(exe)), 0o644); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

func disable() error {
	p, err := desktopFile()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

func enabled() (bool, error) {
	p, err := desktopFile()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
