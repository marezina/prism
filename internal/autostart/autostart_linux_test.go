package autostart

import (
	"os"
	"strings"
	"testing"
)

func TestLinuxAutostart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if on, err := Enabled(); err != nil || on {
		t.Fatalf("Enabled = %v, %v", on, err)
	}
	if err := enable(`/opt/My "PRism"/prism`); err != nil {
		t.Fatal(err)
	}
	p, _ := desktopFile()
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `Exec="/opt/My \"PRism\"/prism" --background`) {
		t.Errorf("desktop file:\n%s", b)
	}
	if on, _ := Enabled(); !on {
		t.Error("not enabled after enable")
	}
	if err := Set(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := Enabled(); on {
		t.Error("still enabled after disable")
	}
	if err := disable(); err != nil {
		t.Errorf("disable twice: %v", err)
	}
}
