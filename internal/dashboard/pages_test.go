package dashboard

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPagesExportBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	secret := "private-prompt-project-account-secret"
	if err := os.WriteFile(filepath.Join(root, "auth.json"), []byte(secret), 0000); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	log := `{"type":"session_meta","payload":{"id":"` + secret + `","cwd":"/` + secret + `"}}` + "\n" +
		`{"type":"turn_context","payload":{"model":"example-model"}}` + "\n" +
		`{"timestamp":"` + stamp + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":20,"total_tokens":120},"last_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":20,"total_tokens":120}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "sessions", secret+".jsonl"), []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "usage.json")
	state := filepath.Join(root, "missing-state")
	if err := exportPages(context.Background(), root, state, out, "tokens,calls,cache", time.UTC); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("example-model")) || bytes.Contains(raw, []byte("models")) {
		t.Fatalf("export leaked non-selected fields: %s", raw)
	}
	s, err := readPagesSnapshot(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tokens == nil || *s.Tokens != 120 || s.Calls == nil || *s.Calls != 1 {
		t.Fatalf("incorrect exported usage: %s", raw)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("export modified private cache")
	}
	if err := exportPages(context.Background(), root, state, out, "tokens,calls,cache", time.UTC); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(out)
	if !bytes.Equal(raw, again) {
		t.Fatal("unchanged data changed exported snapshot")
	}
}

func TestPagesStaticContract(t *testing.T) {
	s, err := readPagesSnapshot("testdata/public-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	for mask := 1; mask < 16; mask++ {
		t.Run(strings.Join(selectedComponents(mask), "-"), func(t *testing.T) {
			input := s
			if mask&1 == 0 {
				input.Tokens = nil
			}
			if mask&2 == 0 {
				input.Calls = nil
			}
			if mask&4 == 0 {
				input.CacheRate = nil
			}
			if mask&8 == 0 {
				input.Models = nil
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "usage.json")
			if err := atomicJSON(path, input); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "site")
			if err := buildPages(path, out); err != nil {
				t.Fatal(err)
			}
			for part := 1; part < 16; part++ {
				for _, format := range []string{"html", "svg"} {
					path := filepath.Join(out, "hub", strings.Join(selectedComponents(part), "-"), "auto."+format)
					raw, err := os.ReadFile(path)
					if part&mask != part {
						if !os.IsNotExist(err) {
							t.Fatal("excluded component is public", path)
						}
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					if part&8 == 0 && bytes.Contains(raw, []byte("example-model")) {
						t.Fatal("model leaked into component", path)
					}
					if format == "html" && !bytes.Contains(raw, []byte(resizeHash)) {
						t.Fatal("static CSP missing", path)
					}
				}
			}
			if err := buildPages(path, out); err == nil {
				t.Fatal("nonempty destination accepted")
			}
			if mask != 15 {
				if _, err := os.Stat(filepath.Join(out, "overview")); !os.IsNotExist(err) {
					t.Fatal("overview leaked omitted fields")
				}
			}
		})
	}
}

func selectedComponents(mask int) []string {
	var names []string
	for i, name := range hubComponents {
		if mask&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return names
}

func TestPagesRejectsPrivateAndInvalidInput(t *testing.T) {
	raw, err := os.ReadFile("testdata/public-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		strings.Replace(string(raw), `"version":1`, `"version":2`, 1),
		strings.Replace(string(raw), `"version":1`, `"version":1,"auth":"secret"`, 1),
		strings.Replace(string(raw), `"calls":240`, `"calls":240,"path":"private"`, 2),
		strings.Replace(string(raw), `"tokens":1250000`, `"tokens":-1`, 1),
		strings.Replace(string(raw), `"82.5%"`, `"NaN%"`, 1),
		strings.Replace(string(raw), `"example-model"`, `"<script>"`, 1),
		strings.Replace(string(raw), `"month":"2026-09"`, `"month":"2026-08"`, 1),
		strings.Repeat(" ", 1<<20+1),
	} {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "output")
		if err := buildPages(path, out); err == nil {
			t.Fatal("invalid input accepted")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("invalid input created output")
		}
	}
	s := pagesSnapshot{Version: 1, Month: "2026-09", CollectedAt: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), Models: new([]modelUsage{})}
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded pagesSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.mask() != 8 {
		t.Fatal("empty models selection lost in round trip")
	}
}
