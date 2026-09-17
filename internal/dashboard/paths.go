package dashboard

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	_ "time/tzdata" // Named zones also work on Windows without a Go installation.
)

func codexHome(override string) (string, error) {
	path := cmp.Or(override, os.Getenv("CODEX_HOME"))
	if path == "" || path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir() // HOME on Unix; USERPROFILE on Windows.
		if err != nil {
			return "", fmt.Errorf("locate Codex home: %w; set CODEX_HOME or -codex-home", err)
		}
		switch {
		case path == "":
			path = filepath.Join(home, ".codex")
		case path == "~":
			path = home
		default:
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}

func localZone(name string) (*time.Location, error) {
	if name == "Local" {
		return time.Local, nil
	}
	if name != "" {
		return time.LoadLocation(name)
	}
	// Preserve an IANA label for remote browser display when Unix provides one.
	// Windows' native Local location comes from the OS and includes DST rules.
	if runtime.GOOS != "windows" {
		if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
			if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
				if loc, err := time.LoadLocation(zone); err == nil {
					return loc, nil
				}
			}
		}
		if raw, err := os.ReadFile("/etc/timezone"); err == nil {
			if loc, err := time.LoadLocation(strings.TrimSpace(string(raw))); err == nil {
				return loc, nil
			}
		}
	}
	return time.Local, nil
}
