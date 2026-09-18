package dashboard

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type metric struct {
	Users   *float64 `json:"users"`
	Threads *float64 `json:"threads"`
	Turns   *float64 `json:"turns"`
	Credits *float64 `json:"credits"`
	Input   *float64 `json:"uncached_text_input_tokens"`
	Cached  *float64 `json:"cached_text_input_tokens"`
	Output  *float64 `json:"text_output_tokens"`
	Total   *float64 `json:"text_total_tokens"`
}
type clientCount struct {
	Client string `json:"client"`
	metric
}
type modelCount struct {
	Model string `json:"model"`
	metric
}
type countDay struct {
	Date    string        `json:"date"`
	Totals  metric        `json:"totals"`
	Clients []clientCount `json:"clients"`
	Models  []modelCount  `json:"models"`
}
type counts struct {
	BalanceUnit *string    `json:"balanceUnit"`
	Data        []countDay `json:"data"`
}

func normalizeCounts(raw []byte) (any, error) {
	var input struct {
		BalanceUnit *string `json:"balance_unit"`
		Data        *[]struct {
			Date    string `json:"date"`
			Totals  metric `json:"totals"`
			Clients []struct {
				ID string `json:"client_id"`
				metric
			} `json:"clients"`
			Models []modelCount `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Data == nil {
		return nil, errors.New("每日统计接口结构发生变化")
	}
	out := counts{BalanceUnit: input.BalanceUnit}
	for _, d := range *input.Data {
		row := countDay{Date: d.Date, Totals: d.Totals, Models: d.Models}
		for _, c := range d.Clients {
			row.Clients = append(row.Clients, clientCount{Client: c.ID, metric: c.metric})
		}
		out.Data = append(out.Data, row)
	}
	return out, nil
}

type modelBreakdown struct {
	Model string   `json:"model"`
	Speed string   `json:"speed"`
	Value *float64 `json:"value"`
}

func normalizeBreakdown(raw []byte) (any, error) {
	var input struct {
		Units *string `json:"units"`
		Data  *[]struct {
			Date     string             `json:"date"`
			Products map[string]float64 `json:"product_surface_usage_values"`
			Models   []struct {
				Model   string   `json:"model"`
				Speed   string   `json:"speed"`
				Credits *float64 `json:"credits"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Data == nil {
		return nil, errors.New("模型用量接口结构发生变化")
	}
	type day struct {
		Date     string             `json:"date"`
		Products map[string]float64 `json:"products"`
		Models   []modelBreakdown   `json:"models"`
	}
	out := struct {
		Units *string `json:"units"`
		Data  []day   `json:"data"`
	}{Units: input.Units}
	for _, d := range *input.Data {
		row := day{Date: d.Date, Products: d.Products}
		for _, m := range d.Models {
			row.Models = append(row.Models, modelBreakdown{m.Model, m.Speed, m.Credits})
		}
		out.Data = append(out.Data, row)
	}
	return out, nil
}

type rateWindow struct {
	UsedPercent float64 `json:"usedPercent"`
	Seconds     *int64  `json:"seconds"`
	ResetsAt    *int64  `json:"resetsAt"`
}
type rawWindow struct {
	UsedPercent *float64 `json:"used_percent"`
	Seconds     *int64   `json:"limit_window_seconds"`
	ResetsAt    *int64   `json:"reset_at"`
}
type rawLimit struct {
	Primary   *rawWindow `json:"primary_window"`
	Secondary *rawWindow `json:"secondary_window"`
}
type rateBucket struct {
	Name      string      `json:"name"`
	Primary   *rateWindow `json:"primary"`
	Secondary *rateWindow `json:"secondary"`
}
type limits struct {
	Plan       *string      `json:"plan"`
	Buckets    []rateBucket `json:"buckets"`
	ResetCount *int         `json:"resetCount"`
}

func normalizeLimits(raw []byte) (any, error) {
	var input struct {
		Plan  *string   `json:"plan_type"`
		Limit *rawLimit `json:"rate_limit"`
		Extra []struct {
			Name  string   `json:"limit_name"`
			Limit rawLimit `json:"rate_limit"`
		} `json:"additional_rate_limits"`
		Resets *struct {
			AvailableCount *int `json:"available_count"`
		} `json:"rate_limit_reset_credits"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, errors.New("额度接口结构发生变化")
	}
	// A null limit is valid; an absent field indicates an incompatible response.
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if _, ok := fields["rate_limit"]; !ok {
		return nil, errors.New("额度接口结构发生变化")
	}
	window := func(w *rawWindow) *rateWindow {
		if w == nil || w.UsedPercent == nil {
			return nil
		}
		return &rateWindow{*w.UsedPercent, w.Seconds, w.ResetsAt}
	}
	bucket := func(name string, r *rawLimit) rateBucket {
		b := rateBucket{Name: name}
		if r != nil {
			b.Primary = window(r.Primary)
			b.Secondary = window(r.Secondary)
		}
		return b
	}
	out := limits{Plan: input.Plan, Buckets: []rateBucket{bucket("Codex", input.Limit)}}
	if input.Resets != nil && input.Resets.AvailableCount != nil && *input.Resets.AvailableCount >= 0 {
		out.ResetCount = input.Resets.AvailableCount
	}
	for _, b := range input.Extra {
		out.Buckets = append(out.Buckets, bucket(b.Name, &b.Limit))
	}
	return out, nil
}

type resetCredit struct {
	ExpiresAt *time.Time `json:"expiresAt"`
}

type resetCredits struct {
	AvailableCount int           `json:"availableCount"`
	Credits        []resetCredit `json:"credits"`
}

func normalizeResetCredits(raw []byte) (any, error) {
	var input struct {
		AvailableCount *int `json:"available_count"`
		Credits        []struct {
			Status    string     `json:"status"`
			ExpiresAt *time.Time `json:"expires_at"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.AvailableCount == nil || *input.AvailableCount < 0 {
		return nil, errors.New("重置次数接口结构发生变化")
	}
	out := resetCredits{AvailableCount: *input.AvailableCount}
	if input.Credits != nil {
		out.Credits = []resetCredit{}
		for _, credit := range input.Credits {
			if credit.Status == "available" {
				out.Credits = append(out.Credits, resetCredit{ExpiresAt: credit.ExpiresAt})
			}
		}
	}
	return out, nil
}

type limitsResult struct {
	remoteResult
	ResetCredits remoteResult `json:"resetCredits"`
}

func (c *accountClient) readLimits(tokens credential, refresh bool) limitsResult {
	var out limitsResult
	var wg sync.WaitGroup
	wg.Go(func() { out.remoteResult = c.read(tokens, "usage", nil, normalizeLimits, refresh) })
	wg.Go(func() {
		out.ResetCredits = c.read(tokens, "rate-limit-reset-credits", nil, normalizeResetCredits, refresh)
	})
	wg.Wait()
	out.RefreshAfterMS = min(out.RefreshAfterMS, out.ResetCredits.RefreshAfterMS)
	return out
}

var errQuotaExpired = errors.New("额度窗口已过期，请刷新额度")

func (l limits) period(seconds int64, now time.Time) (period, error) {
	if seconds == 18000 && l.Plan != nil && strings.HasPrefix(strings.ToLower(*l.Plan), "pro") {
		return period{}, errors.New("Pro 不提供 5 小时统计窗口")
	}
	for _, bucket := range l.Buckets {
		if bucket.Name != "Codex" {
			continue
		}
		for _, w := range []*rateWindow{bucket.Primary, bucket.Secondary} {
			if w == nil || w.Seconds == nil || *w.Seconds != seconds || w.ResetsAt == nil {
				continue
			}
			end := time.Unix(*w.ResetsAt, 0).In(now.Location())
			start := end.Add(-time.Duration(*w.Seconds) * time.Second)
			if !end.After(now) || start.After(now) {
				return period{}, errQuotaExpired
			}
			return period{From: start.UnixMilli(), To: end.UnixMilli(), StartDate: start.Format(time.DateOnly), EndDate: end.Add(-time.Nanosecond).Format(time.DateOnly), Timezone: now.Location().String()}, nil
		}
	}
	return period{}, errors.New("接口未提供此额度窗口")
}

func (c *accountClient) quotaPeriod(seconds int64, refresh bool) (period, error) {
	tokens, err := c.credentials()
	if err != nil {
		return period{}, err
	}
	result := c.read(tokens, "usage", nil, normalizeLimits, refresh)
	resolve := func(result remoteResult) (period, error) {
		if data, ok := result.Data.(limits); ok {
			return data.period(seconds, time.Now())
		}
		if result.Error != nil {
			return period{}, errors.New(*result.Error)
		}
		return period{}, errors.New("无法读取额度窗口，请刷新额度后重试")
	}
	window, err := resolve(result)
	if errors.Is(err, errQuotaExpired) && result.Cached && result.Error == nil && !refresh {
		// The cached reset boundary no longer describes the current window.
		// Revalidate once; an upstream failure must not cause a retry loop.
		result = c.read(tokens, "usage", nil, normalizeLimits, true)
		window, err = resolve(result)
	}
	if err != nil && result.Error != nil {
		return period{}, errors.New(*result.Error)
	}
	return window, err
}

type remoteResult struct {
	Data           any        `json:"data"`
	FetchedAt      *time.Time `json:"fetchedAt"`
	Source         string     `json:"source"`
	Cached         bool       `json:"cached"`
	Error          *string    `json:"error"`
	RefreshAfterMS int64      `json:"refreshAfterMs"`
}

const accountCacheTTL = 30 * time.Minute
const accountRetryDelay = 30 * time.Second

type remoteCache struct {
	result remoteResult
	until  time.Time
}
type accountClient struct {
	mu         sync.Mutex
	cache      map[string]remoteCache
	pending    map[string]chan struct{}
	http       *http.Client
	ctx        context.Context
	root, base string
	now        func() time.Time
}

func newAccountClient(ctx context.Context, root string) *accountClient {
	return &accountClient{ctx: ctx, root: root, now: time.Now, base: "https://chatgpt.com/backend-api/wham/", cache: map[string]remoteCache{}, pending: map[string]chan struct{}{}, http: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type credential struct {
	Access  string `json:"access_token"`
	Account string `json:"account_id"`
}

func (c *accountClient) credentials() (credential, error) {
	var raw struct {
		Tokens credential `json:"tokens"`
	}
	if readJSON(filepath.Join(c.root, "auth.json"), &raw) != nil {
		return credential{}, errors.New("无法读取本机 Codex auth.json，请执行 codex login")
	}
	if raw.Tokens.Access == "" || raw.Tokens.Account == "" {
		return credential{}, errors.New("需要 Codex 的 ChatGPT 登录凭证")
	}
	return raw.Tokens, nil
}
func (c *accountClient) read(tokens credential, path string, params url.Values, normalize func([]byte) (any, error), refresh bool) remoteResult {
	key := fmt.Sprintf("%x:%s:%s", sha256.Sum256([]byte(tokens.Account)), path, params.Encode())
	c.mu.Lock()
	if pending := c.pending[key]; pending != nil {
		c.mu.Unlock()
		select {
		case <-pending:
			return c.read(tokens, path, params, normalize, false)
		case <-c.ctx.Done():
			return remoteResult{Source: path, Error: new("服务正在停止")}
		}
	}
	cached, exists := c.cache[key]
	if exists && !refresh && c.now().Before(cached.until) {
		result := cached.result
		result.RefreshAfterMS = max(1, cached.until.Sub(c.now()).Milliseconds())
		c.mu.Unlock()
		result.Cached = true
		return result
	}
	done := make(chan struct{})
	c.pending[key] = done
	c.mu.Unlock()
	result := remoteResult{Source: path}
	address := c.base + path
	if len(params) > 0 {
		address += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, address, nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+tokens.Access)
		req.Header.Set("ChatGPT-Account-Id", tokens.Account)
		var res *http.Response
		res, err = c.http.Do(req)
		if err != nil {
			err = errors.New("无法连接用量接口，请检查网络或稍后刷新")
		} else {
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				switch res.StatusCode {
				case 401:
					err = errors.New("Codex 登录已过期，请执行 codex login 后刷新")
				case 403:
					err = errors.New("用量接口拒绝访问（403）")
				case 429:
					err = errors.New("请求过于频繁（429），请稍后刷新")
				default:
					err = fmt.Errorf("用量接口暂不可用（%d）", res.StatusCode)
				}
			} else {
				var raw []byte
				raw, err = io.ReadAll(io.LimitReader(res.Body, 16<<20))
				if err == nil {
					result.Data, err = normalize(raw)
				}
				if err == nil {
					result.FetchedAt = new(c.now())
				}
			}
		}
	}
	if err != nil {
		if exists {
			result = cached.result
			result.Cached = true
		}
		result.Error = new(err.Error())
	}
	delay := accountCacheTTL
	if err != nil {
		delay = accountRetryDelay
	}
	result.RefreshAfterMS = delay.Milliseconds()
	c.mu.Lock()
	// Failed reads are retained too, so concurrent waiters share the same outcome.
	c.cache[key] = remoteCache{result: result, until: c.now().Add(delay)}
	if len(c.cache) > 48 {
		for k := range c.cache {
			if k != key {
				delete(c.cache, k)
				break
			}
		}
	}
	delete(c.pending, key)
	close(done)
	c.mu.Unlock()
	return result
}

type accountUsage struct {
	Range     period       `json:"range"`
	Limits    remoteResult `json:"limits"`
	Counts    remoteResult `json:"counts"`
	Breakdown remoteResult `json:"breakdown"`
	Error     string       `json:"error,omitempty"`
}

func (c *accountClient) usage(window period, refresh bool) accountUsage {
	tokens, err := c.credentials()
	if err != nil {
		return accountUsage{Range: window, Error: err.Error()}
	}
	params := url.Values{"start_date": {window.StartDate}, "end_date": {window.EndDate}, "group_by": {"day"}}
	personal := params.Clone()
	personal.Set("workspace_user", "true")
	out := accountUsage{Range: window}
	var wg sync.WaitGroup
	wg.Go(func() { out.Limits = c.read(tokens, "usage", nil, normalizeLimits, refresh) })
	wg.Go(func() {
		out.Counts = c.read(tokens, "analytics/daily-workspace-usage-counts", personal, normalizeCounts, refresh)
	})
	wg.Go(func() {
		out.Breakdown = c.read(tokens, "usage/daily-token-usage-breakdown", params, normalizeBreakdown, refresh)
	})
	wg.Wait()
	return out
}
