package dashboard

import (
	"context"
	json "encoding/json/v2"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func tokenLine(t *testing.T, stamp string, total tokens, last *tokens) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"timestamp": stamp, "type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": total, "last_token_usage": last}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func writeFixture(t *testing.T, root string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "example.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestTokens(t *testing.T) {
	first := tokens{Input: 100, Cached: 60, Output: 10, Reasoning: 5, Total: 110}
	second := tokens{Input: 300, Cached: 140, Output: 50, Reasoning: 20, Total: 350}
	path := writeFixture(t, t.TempDir(), `{"type":"turn_context","payload":{"model":"gpt-6-astra"}}`, tokenLine(t, "2026-09-10T01:00:00Z", first, &first), tokenLine(t, "2026-09-10T01:00:01Z", first, &first), tokenLine(t, "2026-09-10T02:00:00Z", second, nil), `{"type":"token_count",unfinished`)
	parsed, err := parseSession(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 2 || parsed.Malformed != 1 {
		t.Fatalf("bad records: %+v", parsed)
	}
	start, _ := time.Parse(time.RFC3339, "2026-09-10T02:00:00Z")
	out := aggregateLocal(&snapshot{Files: map[string]session{"s": parsed}}, period{From: start.UnixMilli(), To: start.Add(time.Hour).UnixMilli()}, priceBook{}, currency{"USD", 1})
	if out.Calls != 1 || out.Tokens != 240 || out.Input != 120 || out.Cached != 80 || out.Output != 40 || out.Reasoning != 15 {
		t.Fatalf("unexpected usage: %+v", out)
	}
	if out.Cost != nil || out.UnpricedCalls != 1 {
		t.Fatal("unknown price must stay unknown")
	}
	reset := tokens{Input: 20, Cached: 5, Write: 4, Output: 3, Total: 23}
	path = writeFixture(t, t.TempDir(), tokenLine(t, "2026-09-10T01:00:00Z", first, &first), tokenLine(t, "2026-09-10T02:00:00Z", reset, &reset))
	parsed, err = parseSession(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Events[1].Tokens != 23 || parsed.Events[1].Input != 11 || parsed.Events[1].Write != 4 {
		t.Fatal("reset or cache write accounting failed")
	}
}
func TestRecentWeekIncludesToday(t *testing.T) {
	for _, test := range []struct{ zone, now, start string }{
		{"Asia/Hong_Kong", "2026-09-16T23:30:00+08:00", "2026-09-10T00:00:00+08:00"},
		{"Asia/Hong_Kong", "2026-09-16T00:00:00+08:00", "2026-09-10T00:00:00+08:00"},
		{"Asia/Hong_Kong", "2026-03-02T12:00:00+08:00", "2026-02-24T00:00:00+08:00"},
		{"America/New_York", "2026-03-10T12:00:00-04:00", "2026-03-04T00:00:00-05:00"},
	} {
		loc, err := time.LoadLocation(test.zone)
		if err != nil {
			t.Fatal(err)
		}
		now, err := time.Parse(time.RFC3339, test.now)
		if err != nil {
			t.Fatal(err)
		}
		now = now.In(loc)
		for _, name := range []string{"week", ""} {
			r, err := periodRange(name, "", now)
			if err != nil {
				t.Fatal(err)
			}
			if start := time.UnixMilli(r.From).In(loc).Format(time.RFC3339); start != test.start || r.To != now.UnixMilli() || r.StartDate != test.start[:10] || r.EndDate != test.now[:10] {
				t.Fatalf("%s %s: unexpected week: %+v", test.zone, test.now, r)
			}
			out := aggregateLocal(&snapshot{Files: map[string]session{"s": {Events: []event{
				{Timestamp: r.From - 1, Tokens: 100}, {Timestamp: r.From, Tokens: 10}, {Timestamp: r.To - 1, Tokens: 20}, {Timestamp: r.To, Tokens: 100},
			}}}}, r, priceBook{}, currency{"USD", 1})
			if out.Tokens != 30 || out.Calls != 2 {
				t.Fatalf("week boundary events: %+v", out)
			}
		}
	}
}

func TestResponseUsageOwnsTurn(t *testing.T) {
	old := tokens{Input: 100, Output: 10, Total: 110}
	record := func(response, stamp string, usage tokens) string {
		raw, err := json.Marshal(map[string]any{"timestamp": stamp, "type": "token_usage_record", "payload": map[string]any{"turn_id": "new", "response_id": response, "usage": usage}})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	path := writeFixture(t, t.TempDir(),
		`{"type":"turn_context","payload":{"turn_id":"old","model":"old-model"}}`,
		tokenLine(t, "2026-09-10T01:00:00Z", old, &old),
		record("early-compact", "2026-09-10T01:59:00Z", tokens{Input: 35, Output: 5, Total: 40}),
		`{"type":"turn_context","payload":{"turn_id":"new","model":"new-model"}}`,
		record("answer", "2026-09-10T02:00:00Z", old),
		tokenLine(t, "2026-09-10T02:00:01Z", tokens{Input: 200, Output: 20, Total: 220}, &old),
		record("answer", "2026-09-10T02:00:00Z", tokens{Input: 200, Cached: 100, Output: 20, Total: 220}),
		record("compact", "2026-09-10T02:01:00Z", tokens{Input: 300, Cached: 200, Output: 30, Total: 330}),
		`{"type":"compacted","payload":{}}`,
		`{"type":"turn_context","payload":{"turn_id":"later","model":"later-model"}}`,
		record("compact", "2026-09-10T02:01:00Z", tokens{Input: 300, Cached: 200, Output: 30, Total: 330}),
		`{"type":"token_usage_record","unfinished`,
	)
	parsed, err := parseSession(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range parsed.Events {
		total += e.Tokens
	}
	if total != 700 || len(parsed.Events) != 4 || parsed.Malformed != 1 || parsed.Events[0].Model != "old-model" || parsed.Events[1].Model != "new-model" || parsed.Events[3].Model != "new-model" {
		t.Fatalf("response deduplication, compaction or legacy turn lost: %+v", parsed)
	}
}

func TestQuotaPeriod(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for _, seconds := range []int64{18000, 604800} {
		reset := now.Add(2 * time.Hour).Unix()
		l := limits{Plan: new("plus"), Buckets: []rateBucket{{Name: "Codex", Secondary: &rateWindow{Seconds: new(seconds), ResetsAt: new(reset)}}}}
		r, err := l.period(seconds, now)
		if err != nil || r.From != (reset-seconds)*1000 || r.To != reset*1000 {
			t.Fatalf("window must follow upstream reset: %+v %v", r, err)
		}
		l.Plan = new("pro")
		if _, err := l.period(18000, now); err == nil {
			t.Fatal("Pro accepted five-hour window")
		}
		l.Plan = new("plus")
		if _, err := l.period(seconds, now.Add(3*time.Hour)); err == nil {
			t.Fatal("expired window accepted")
		}
	}
	if _, err := (limits{}).period(18000, now); err == nil {
		t.Fatal("invented missing quota window")
	}
}

func TestOldCacheRebuilt(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	store, err := newLocalStore(t.Context(), root, state)
	if err != nil {
		t.Fatal(err)
	}
	old := snapshot{Version: 1, Source: store.source, FetchedAt: time.Now(), Files: map[string]session{}}
	if err := atomicJSON(store.path, old); err != nil {
		t.Fatal(err)
	}
	restarted, err := newLocalStore(t.Context(), root, state)
	if err != nil {
		t.Fatal(err)
	}
	if data, _, _ := restarted.view(); data != nil {
		t.Fatal("obsolete accounting cache reused")
	}
	<-restarted.refresh()
	if data, _, _ := restarted.view(); data == nil || data.Version != 2 {
		t.Fatal("cache not rebuilt")
	}
}

func TestPeriods(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Hong_Kong")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 16, 15, 30, 0, 0, loc)
	for _, test := range []struct{ period, end, from, to string }{{"yesterday", "", "2026-09-15", "2026-09-15"}, {"this_week", "", "2026-09-14", "2026-09-16"}, {"last_week", "", "2026-09-07", "2026-09-13"}, {"last_month", "", "2026-08-01", "2026-08-31"}, {"custom7d", "2026-09-16", "2026-09-10", "2026-09-16"}} {
		r, err := periodRange(test.period, test.end, now)
		if err != nil || r.StartDate != test.from || r.EndDate != test.to {
			t.Fatalf("%s: %+v %v", test.period, r, err)
		}
	}
	r, err := periodRange("custom5", "2026-09-16T18:00", now)
	if err != nil || r.To-r.From != 5*time.Hour.Milliseconds() {
		t.Fatal(r, err)
	}
	for _, end := range []string{"2026-02-31", "2026-9-16", "bad"} {
		if _, err := periodRange("custom7d", end, now); err == nil {
			t.Fatal("accepted invalid date", end)
		}
	}
	out := aggregateLocal(&snapshot{Files: map[string]session{"s": {Events: []event{{Timestamp: r.To, Tokens: 10}}}}}, r, priceBook{}, currency{"USD", 1})
	if out.Calls != 0 {
		t.Fatal("end must be exclusive")
	}
}
func TestAstraPricing(t *testing.T) {
	prices, err := loadPrices(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	e := event{Model: "gpt-6-astra", Input: 1_000_000, Cached: 1_000_000, Write: 1_000_000, Output: 1_000_000}
	cost, known := prices.cost(e)
	if !known || math.Abs(cost-73.5) > 1e-9 {
		t.Fatalf("standard token valuation: %v %v", cost, known)
	}
	e.Model = "future-model"
	if _, known = prices.cost(e); known {
		t.Fatal("invented future model price")
	}
	override := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(override, []byte(`{"updatedAt":"2026-10-01","models":{"future-model":{"inputCostPerToken":0.000001,"cacheReadCostPerToken":0,"cacheWriteCostPerToken":0,"outputCostPerToken":0.000002}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	prices, err = loadPrices(t.TempDir(), override)
	if err != nil {
		t.Fatal(err)
	}
	cost, known = prices.cost(e)
	if !known || cost != 3 {
		t.Fatal("structured price override", cost, known)
	}
}
func TestCachePersistenceAndSchedule(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := tokens{Input: 100, Output: 10, Total: 110}
	writeFixture(t, root, tokenLine(t, "2026-09-10T01:00:00Z", first, &first), `{"type":"response_item","payload":{"text":"private-conversation-sentinel"}}`)
	store, err := newLocalStore(ctx, root, state)
	if err != nil {
		t.Fatal(err)
	}
	<-store.refresh()
	data, busy, message := store.view()
	if data == nil || busy || message != "" {
		t.Fatalf("cache refresh failed %v %s", busy, message)
	}
	raw, err := os.ReadFile(filepath.Join(state, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-conversation-sentinel") {
		t.Fatal("conversation text leaked into cache")
	}
	info, _ := os.Stat(filepath.Join(state, "sessions.json"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("cache permissions")
	}
	restart, err := newLocalStore(ctx, root, state)
	if err != nil {
		t.Fatal(err)
	}
	saved, _, _ := restart.view()
	if saved == nil || !saved.FetchedAt.Equal(data.FetchedAt) {
		t.Fatal("disk cache not restored")
	}
	other, err := newLocalStore(ctx, t.TempDir(), state)
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, _ := other.view()
	if foreign != nil {
		t.Fatal("cache crossed CODEX_HOME boundary")
	}
	second := tokens{Input: 200, Output: 20, Total: 220}
	writeFixture(t, root, tokenLine(t, "2026-09-10T01:00:00Z", first, &first), tokenLine(t, "2026-09-10T02:00:00Z", second, &first))
	store.run(30 * time.Millisecond)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("scheduled cache refresh did not run")
		case <-time.After(10 * time.Millisecond):
		}
		updated, _, _ := store.view()
		if len(updated.Files[filepath.Join("sessions", "example.jsonl")].Events) == 2 {
			break
		}
	}
	// A failed refresh must keep the last good persisted snapshot.
	cancel()
	store.wg.Wait()
	before, _, _ := store.view()
	<-store.refresh()
	after, _, message := store.view()
	if !before.FetchedAt.Equal(after.FetchedAt) || message == "" {
		t.Fatal("failed refresh replaced good data")
	}
}
