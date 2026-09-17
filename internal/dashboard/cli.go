package dashboard

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Run executes a CLI command, or serves the dashboard when args is empty.
func Run(args []string, version string) error {
	if len(args) == 0 {
		return run()
	}
	if args[0] == "version" || args[0] == "--version" {
		if len(args) != 1 {
			return errors.New("unexpected version arguments")
		}
		fmt.Println(version)
		return nil
	}
	if args[0] == "help" || args[0] == "--help" {
		fmt.Println("codex-dashboard [export|sync|build-pages|version]\nNo command: start the local dashboard. Use COMMAND -h for options.")
		return nil
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	switch args[0] {
	case "export", "sync":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		root := flags.String("codex-home", "", "Codex directory (default CODEX_HOME or the current user's .codex)")
		state := flags.String("state", cmp.Or(os.Getenv("DASHBOARD_STATE_DIR"), filepath.Join(filepath.Dir(exe), ".state")), "existing local cache directory (read only)")
		var output, repo, logPath string
		if args[0] == "sync" {
			flags.StringVar(&repo, "repo", ".", "Git working tree; sync owns only site/usage.json")
			flags.StringVar(&logPath, "log", "", "append sync output to this file (use .state/sync.log for scheduled jobs)")
		} else {
			flags.StringVar(&output, "out", "site/usage.json", "public snapshot destination")
		}
		components := flags.String("components", "tokens,calls,cache", "public fields; models is opt-in")
		zone := flags.String("timezone", os.Getenv("TZ"), "month boundary timezone (default system local time)")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected command arguments")
		}
		loc, err := localZone(*zone)
		if err != nil {
			return err
		}
		resolved, err := codexHome(*root)
		if err != nil {
			return err
		}
		if args[0] == "sync" {
			if logPath != "" {
				if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
					return err
				}
				file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				defer file.Close()
				previous := log.Writer()
				log.SetOutput(io.MultiWriter(previous, file))
				defer log.SetOutput(previous)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			err := syncSnapshot(ctx, repo, func(output string) error {
				return exportPages(ctx, resolved, *state, output, *components, loc)
			})
			if err != nil {
				log.Printf("Sync failed: %v", err)
			}
			return err
		}
		return exportPages(context.Background(), resolved, *state, output, *components, loc)
	case "build-pages":
		input := flags.String("input", "site/usage.json", "public snapshot")
		output := flags.String("out", "_site", "empty static output directory")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected build-pages arguments")
		}
		return buildPages(*input, *output)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
