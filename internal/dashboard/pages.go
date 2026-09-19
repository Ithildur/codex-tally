package dashboard

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This is the entire publication boundary. Never serialize snapshot or session.
type pagesSnapshot struct {
	Version     int           `json:"version"`
	CollectedAt time.Time     `json:"collectedAt"`
	Month       string        `json:"month"`
	Tokens      *int64        `json:"tokens,omitzero"`
	Calls       *int64        `json:"calls,omitzero"`
	CacheRate   *string       `json:"cacheRate,omitzero"`
	Models      *[]modelUsage `json:"models,omitzero"`
}

var modelName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,95}$`)

func (s pagesSnapshot) mask() int {
	mask := 0
	if s.Tokens != nil {
		mask |= 1
	}
	if s.Calls != nil {
		mask |= 2
	}
	if s.CacheRate != nil {
		mask |= 4
	}
	if s.Models != nil {
		mask |= 8
	}
	return mask
}

func (s pagesSnapshot) validate() error {
	month, err := time.Parse("2006-01", s.Month)
	if err != nil || month.Year() < 2000 || s.Version != 1 || s.CollectedAt.IsZero() || s.CollectedAt.Format("2006-01") != s.Month || s.mask() == 0 {
		return errors.New("invalid public snapshot metadata")
	}
	for _, count := range []*int64{s.Tokens, s.Calls} {
		if count != nil && *count < 0 {
			return errors.New("negative public count")
		}
	}
	if s.CacheRate != nil && *s.CacheRate != "—" {
		text, ok := strings.CutSuffix(*s.CacheRate, "%")
		n, err := strconv.ParseFloat(text, 64)
		if !ok || err != nil || !(n >= 0 && n <= 100) {
			return errors.New("invalid cache rate")
		}
	}
	if s.Models != nil {
		if len(*s.Models) > 256 {
			return errors.New("too many public models")
		}
		seen := map[string]bool{}
		for _, m := range *s.Models {
			if !modelName.MatchString(m.Model) || m.Tokens < 0 || m.Calls < 0 || seen[m.Model] {
				return errors.New("invalid public model")
			}
			seen[m.Model] = true
		}
	}
	return nil
}

func readPagesSnapshot(path string) (pagesSnapshot, error) {
	var s pagesSnapshot
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return s, err
	}
	return decodePagesSnapshot(raw)
}

func decodePagesSnapshot(raw []byte) (pagesSnapshot, error) {
	var s pagesSnapshot
	if len(raw) > 1<<20 {
		return s, errors.New("public snapshot exceeds 1 MiB")
	}
	if err := json.Unmarshal(raw, &s, json.RejectUnknownMembers(true)); err != nil {
		return s, err
	}
	return s, s.validate()
}

func exportPages(ctx context.Context, root, state, output, components string, loc *time.Location) error {
	mask, err := componentMask(components)
	if err != nil {
		return err
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return errors.New("Codex directory does not exist")
	}
	store, err := newLocalStore(ctx, root, state)
	if err != nil {
		return err
	}
	old, _, _ := store.view()
	data, err := store.scan(old)
	if err != nil {
		return err
	}
	if data.Unreadable != 0 {
		return fmt.Errorf("%d session files unreadable; export not replaced", data.Unreadable)
	}
	// The collector is local-only: no auth files, account API, or HTTP server.
	data.FetchedAt = data.FetchedAt.In(loc)
	view := monthlyPublicUsage(data, loc)
	s := pagesSnapshot{Version: 1, CollectedAt: data.FetchedAt, Month: data.FetchedAt.Format("2006-01")}
	if mask&1 != 0 {
		s.Tokens = &view.Tokens
	}
	if mask&2 != 0 {
		s.Calls = &view.Calls
	}
	if mask&4 != 0 {
		s.CacheRate = &view.CacheRate
	}
	if mask&8 != 0 {
		s.Models = &view.Models
	}
	if err := s.validate(); err != nil {
		return err
	}
	// Keep the original collection timestamp when the published values have not
	// changed. Offline retries then produce no gratuitous commits or deploys.
	if previous, err := readPagesSnapshot(output); err == nil {
		candidate := s
		candidate.CollectedAt = previous.CollectedAt
		a, _ := json.Marshal(candidate)
		b, _ := json.Marshal(previous)
		if bytes.Equal(a, b) {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return atomicJSON(output, s)
}

func buildPages(input, output string) error {
	s, err := readPagesSnapshot(input)
	if err != nil {
		return err
	}
	view := publicUsage{Month: strings.ReplaceAll(s.Month, "-", "."), Updated: s.CollectedAt.Format("2006-01-02 15:04 -07:00"), CacheRate: "—"}
	if s.Tokens != nil {
		view.Tokens = *s.Tokens
	}
	if s.Calls != nil {
		view.Calls = *s.Calls
	}
	if s.CacheRate != nil {
		view.CacheRate = *s.CacheRate
	}
	if s.Models != nil {
		view.Models = *s.Models
	}
	assets, err := renderPublicAssets(view, s.mask())
	if err != nil {
		return err
	}
	// A fresh destination prevents removed components surviving a later export.
	if entries, err := os.ReadDir(output); err == nil && len(entries) != 0 {
		return errors.New("output directory must be empty; use a new directory or remove the previous build")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	write := func(name string, raw []byte) error {
		path := filepath.Join(output, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, raw, 0644)
	}
	for key, raw := range assets {
		if strings.HasSuffix(key, ".html") {
			csp := `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src ` + resizeHash + `; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">`
			raw = bytes.Replace(raw, []byte(`<meta charset="utf-8">`), []byte(`<meta charset="utf-8">`+csp), 1)
		}
		if err := write(key, raw); err != nil {
			return err
		}
	}
	for name, source := range map[string]string{"index.html": "pages.html", "pages.js": "pages.js", "pages.css": "pages.css", "embed.js": "embed.js", "favicon.svg": "favicon.svg", "i18n.js": "i18n.js", "i18n.json": "i18n.json"} {
		raw, err := webFiles.ReadFile("public/" + source)
		if err != nil {
			return err
		}
		if err := write(name, raw); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := write("usage.json", raw); err != nil {
		return err
	}
	return write(".nojekyll", nil)
}
