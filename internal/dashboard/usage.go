package dashboard

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

type period struct {
	From      int64  `json:"from"`
	To        int64  `json:"to"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Timezone  string `json:"timezone"`
}

func periodRange(name, end string, now time.Time) (period, error) {
	loc := now.Location()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	to := now
	monday := from.AddDate(0, 0, -(int(from.Weekday())+6)%7)
	switch name {
	case "today":
	case "yesterday":
		to = from
		from = from.AddDate(0, 0, -1)
	case "24":
		from = now.Add(-24 * time.Hour)
	case "week", "":
		from = from.AddDate(0, 0, -6)
	case "this_week":
		from = monday
	case "last_week":
		to = monday
		from = monday.AddDate(0, 0, -7)
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	case "last_month":
		to = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		from = to.AddDate(0, -1, 0)
	case "custom5", "custom7d":
		layout := "2006-01-02T15:04"
		if name == "custom7d" {
			layout = time.DateOnly
		}
		parsed, err := time.ParseInLocation(layout, end, loc)
		if err != nil || parsed.Format(layout) != end {
			return period{}, errors.New("窗口结束时间无效")
		}
		to = parsed
		if name == "custom5" {
			from = to.Add(-5 * time.Hour)
		} else {
			to = to.AddDate(0, 0, 1)
			from = to.AddDate(0, 0, -7)
		}
	default:
		return period{}, errors.New("不支持的统计周期")
	}
	endDate := to.Add(-time.Nanosecond).Format(time.DateOnly)
	if name == "week" || name == "" {
		endDate = now.Format(time.DateOnly)
	}
	return period{From: from.UnixMilli(), To: to.UnixMilli(), StartDate: from.Format(time.DateOnly), EndDate: endDate, Timezone: loc.String()}, nil
}

type tokens struct {
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	Write     int64 `json:"cache_write_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
	Total     int64 `json:"total_tokens"`
}
type event struct {
	Timestamp int64  `json:"timestamp"`
	Model     string `json:"model"`
	Input     int64  `json:"input"`
	Cached    int64  `json:"cached"`
	Write     int64  `json:"write"`
	Output    int64  `json:"output"`
	Reasoning int64  `json:"reasoning"`
	Tokens    int64  `json:"tokens"`
	// Full input (including cache) of one response; nil for cumulative-only logs.
	RequestInput *int64 `json:"requestInput,omitempty"`
}
type session struct {
	Size      int64   `json:"size"`
	Modified  int64   `json:"modified"`
	Events    []event `json:"events"`
	Malformed int     `json:"malformed"`
}

func parseSession(ctx context.Context, path string) (session, error) {
	f, err := os.Open(path)
	if err != nil {
		return session{}, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 256*1024)
	model := "unknown"
	turn := ""
	models := map[string]string{}
	type reading struct {
		event
		turn string
	}
	var legacy []reading
	modern := map[string]reading{}
	modernTurns := map[string]bool{}
	var prev *tokens
	result := session{}
	for {
		if err := ctx.Err(); err != nil {
			return session{}, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 && (bytes.Contains(line, []byte(`"token_count"`)) || bytes.Contains(line, []byte(`"token_usage_record"`)) || bytes.Contains(line, []byte(`"turn_context"`)) || bytes.Contains(line, []byte(`"session_meta"`))) {
			var row struct {
				Timestamp string `json:"timestamp"`
				Type      string `json:"type"`
				Payload   struct {
					Type     string  `json:"type"`
					Model    string  `json:"model"`
					Turn     string  `json:"turn_id"`
					Response string  `json:"response_id"`
					Usage    *tokens `json:"usage"`
					Info     *struct {
						Total *tokens `json:"total_token_usage"`
						Last  *tokens `json:"last_token_usage"`
					} `json:"info"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &row) != nil {
				result.Malformed++
			} else {
				p := row.Payload
				if row.Type == "turn_context" || row.Type == "session_meta" {
					if p.Turn != "" {
						turn = p.Turn
					}
					if p.Model != "" {
						model = p.Model
					}
					models[turn] = model
				} else if row.Type == "token_usage_record" && p.Usage != nil && p.Response != "" {
					id := cmp.Or(p.Turn, turn)
					e, err := usageEvent(row.Timestamp, cmp.Or(p.Model, models[id], "unknown"), *p.Usage)
					if err == nil {
						e.RequestInput = new(p.Usage.Input)
						modern[p.Response] = reading{e, id}
						modernTurns[id] = true
					}
				} else if row.Type == "event_msg" && p.Type == "token_count" && p.Info != nil && p.Info.Total != nil {
					current := *p.Info.Total
					duplicate := prev != nil && *prev == current
					reset := prev != nil && current.Total < prev.Total
					delta := current
					if p.Info.Last != nil {
						delta = *p.Info.Last
					} else if prev != nil && !reset {
						delta = tokens{Input: current.Input - prev.Input, Cached: current.Cached - prev.Cached, Write: current.Write - prev.Write, Output: current.Output - prev.Output, Reasoning: current.Reasoning - prev.Reasoning}
					}
					prev = new(current)
					e, err := usageEvent(row.Timestamp, model, delta)
					if p.Info.Last != nil {
						e.RequestInput = new(delta.Input)
					}
					if !duplicate && !(reset && p.Info.Last == nil) && err == nil {
						if e.Tokens > 0 {
							legacy = append(legacy, reading{e, turn})
						}
					}
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return session{}, readErr
		}
	}
	// New records own accounting for their turn, including compaction calls.
	// Legacy-only turns remain readable when a session spans a client upgrade.
	for _, r := range legacy {
		if !modernTurns[r.turn] {
			result.Events = append(result.Events, r.event)
		}
	}
	for _, r := range modern {
		if r.Tokens > 0 {
			if r.Model == "unknown" {
				r.Model = cmp.Or(models[r.turn], "unknown")
			}
			result.Events = append(result.Events, r.event)
		}
	}
	slices.SortFunc(result.Events, func(a, b event) int { return cmp.Compare(a.Timestamp, b.Timestamp) })
	return result, nil
}

func usageEvent(timestamp, model string, usage tokens) (event, error) {
	stamp, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return event{}, err
	}
	cached := min(max(0, usage.Cached), max(0, usage.Input))
	write := min(max(0, usage.Write), max(0, usage.Input-cached))
	e := event{Timestamp: stamp.UnixMilli(), Model: model, Input: max(0, usage.Input-cached-write), Cached: cached, Write: write, Output: max(0, usage.Output), Reasoning: max(0, usage.Reasoning)}
	e.Tokens = e.Input + e.Cached + e.Write + e.Output
	return e, nil
}

type dayUsage struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}
type modelUsage struct {
	Model  string `json:"model"`
	Calls  int    `json:"calls"`
	Tokens int64  `json:"tokens"`
}
type localUsage struct {
	Calls            int          `json:"calls"`
	Sessions         int          `json:"sessions"`
	Input            int64        `json:"input"`
	Cached           int64        `json:"cached"`
	Write            int64        `json:"write"`
	Output           int64        `json:"output"`
	Reasoning        int64        `json:"reasoning"`
	Tokens           int64        `json:"tokens"`
	KnownCostUSD     float64      `json:"knownCostUsd"`
	UnpricedCalls    int          `json:"unpricedCalls"`
	Cost             *float64     `json:"cost"`
	CacheRate        *float64     `json:"cacheRate"`
	Range            period       `json:"range"`
	Hours            [7][24]int   `json:"hours"`
	Currency         currency     `json:"currency"`
	ActiveDays       int          `json:"activeDays"`
	BusiestHour      *int         `json:"busiestHour"`
	Daily            []dayUsage   `json:"daily"`
	Models           []modelUsage `json:"models"`
	MissingPrices    []string     `json:"missingPrices"`
	Files            int          `json:"files"`
	Unreadable       int          `json:"unreadable"`
	Malformed        int          `json:"malformed"`
	FetchedAt        time.Time    `json:"fetchedAt"`
	CacheReady       bool         `json:"cacheReady"`
	Refreshing       bool         `json:"refreshing"`
	Error            string       `json:"error,omitempty"`
	PricingUpdatedAt string       `json:"pricingUpdatedAt"`
}

func aggregateLocal(data *snapshot, window period, prices priceBook, curr currency) localUsage {
	out := localUsage{Range: window, Currency: curr, CacheReady: true, Files: len(data.Files), Unreadable: data.Unreadable, FetchedAt: data.FetchedAt, PricingUpdatedAt: prices.UpdatedAt}
	daily := map[string]int64{}
	models := map[string]modelUsage{}
	missing := map[string]bool{}
	for _, s := range data.Files {
		active := false
		out.Malformed += s.Malformed
		for _, e := range s.Events {
			if e.Timestamp < window.From || e.Timestamp >= window.To {
				continue
			}
			active = true
			out.Calls++
			out.Input += e.Input
			out.Cached += e.Cached
			out.Write += e.Write
			out.Output += e.Output
			out.Reasoning += e.Reasoning
			out.Tokens += e.Tokens
			date := time.UnixMilli(e.Timestamp).In(time.Local)
			out.Hours[(int(date.Weekday())+6)%7][date.Hour()]++
			daily[date.Format(time.DateOnly)] += e.Tokens
			m := models[e.Model]
			m.Model = e.Model
			m.Calls++
			m.Tokens += e.Tokens
			models[e.Model] = m
			cost, known := prices.cost(e)
			if known {
				out.KnownCostUSD += cost
			} else {
				out.UnpricedCalls++
				missing[e.Model] = true
			}
		}
		if active {
			out.Sessions++
		}
	}
	if out.UnpricedCalls == 0 {
		out.Cost = new(out.KnownCostUSD * curr.Rate)
	}
	if input := out.Input + out.Cached + out.Write; input > 0 {
		out.CacheRate = new(float64(out.Cached) / float64(input) * 100)
	}
	out.ActiveDays = len(daily)
	if out.Calls > 0 {
		busiest, best := 0, 0
		for h := range 24 {
			n := 0
			for d := range 7 {
				n += out.Hours[d][h]
			}
			if n > best {
				busiest, best = h, n
			}
		}
		out.BusiestHour = new(busiest)
	}
	for date, n := range daily {
		out.Daily = append(out.Daily, dayUsage{date, n})
	}
	for _, m := range models {
		out.Models = append(out.Models, m)
	}
	for model := range missing {
		out.MissingPrices = append(out.MissingPrices, model)
	}
	slices.SortFunc(out.Daily, func(a, b dayUsage) int { return strings.Compare(a.Date, b.Date) })
	slices.SortFunc(out.Models, func(a, b modelUsage) int {
		return cmp.Or(cmp.Compare(b.Tokens, a.Tokens), strings.Compare(a.Model, b.Model))
	})
	slices.Sort(out.MissingPrices)
	return out
}

func readJSON(path string, into any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("解析 %s: %w", path, err)
	}
	return nil
}
