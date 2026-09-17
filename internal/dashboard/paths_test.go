package dashboard

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCodexHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "用户 home")
	key := "HOME"
	if runtime.GOOS == "windows" {
		key = "USERPROFILE"
	}
	t.Setenv(key, home)
	t.Setenv("CODEX_HOME", "")
	got, err := codexHome("")
	if err != nil || got != filepath.Join(home, ".codex") {
		t.Fatalf("default: %q, %v", got, err)
	}
	custom := filepath.Join(t.TempDir(), "custom")
	t.Setenv("CODEX_HOME", custom)
	got, err = codexHome("")
	if err != nil || got != custom {
		t.Fatalf("environment: %q, %v", got, err)
	}
	got, err = codexHome("~/.codex")
	if err != nil || got != filepath.Join(home, ".codex") {
		t.Fatalf("flag precedence / tilde: %q, %v", got, err)
	}
	got, err = codexHome("relative")
	want, _ := filepath.Abs("relative")
	if err != nil || got != want {
		t.Fatalf("relative: %q, %v", got, err)
	}
	// An explicit directory works without a usable login profile (task runners).
	t.Setenv(key, "")
	got, err = codexHome(custom)
	if err != nil || got != custom {
		t.Fatalf("explicit without home: %q, %v", got, err)
	}
}

func TestLocalZone(t *testing.T) {
	loc, err := localZone("Local")
	if err != nil || loc != time.Local {
		t.Fatal("system timezone was replaced")
	}
	// Named zone support is embedded, including on machines without Go/tzdata.
	loc, err = localZone("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	_, winter := time.Date(2026, 1, 1, 12, 0, 0, 0, loc).Zone()
	_, summer := time.Date(2026, 7, 1, 12, 0, 0, 0, loc).Zone()
	if winter != -5*3600 || summer != -4*3600 {
		t.Fatal("DST rules unavailable")
	}
	if _, err := localZone("Not/AZone"); err == nil {
		t.Fatal("invalid zone accepted")
	}
}
