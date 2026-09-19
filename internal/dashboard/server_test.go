package dashboard

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testApplication(t *testing.T) *application {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	app, err := newApplication(ctx, config{Host: "127.0.0.1", Port: "4318", Root: t.TempDir(), State: t.TempDir(), Home: t.TempDir()}, "test-password-never-use-in-production")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); app.local.wg.Wait() })
	return app
}
func TestLocalPollingRefresh(t *testing.T) {
	app := testApplication(t)
	if err := atomicJSON(filepath.Join(app.config.Root, "auth.json"), map[string]any{"tokens": credential{"test-access", "test-account"}}); err != nil {
		t.Fatal(err)
	}
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		fmt.Fprintf(w, `{"plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_at":%d}}}`, time.Now().Add(24*time.Hour).Unix())
	}))
	defer upstream.Close()
	app.account.base = upstream.URL + "/"
	poll := func() localUsage {
		t.Helper()
		w := httptest.NewRecorder()
		app.localUsage(w, httptest.NewRequest("GET", "/api/local?period=quota7&refresh=local", nil))
		var usage localUsage
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &usage); err != nil {
			t.Fatal(err)
		}
		return usage
	}
	if poll().Calls != 0 {
		t.Fatal("new store should be empty")
	}
	dir := filepath.Join(app.config.Root, "sessions")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "live.jsonl")
	log := fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120},"last_token_usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}}}`, time.Now().Add(-time.Second).Format(time.RFC3339Nano)) + "\n"
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	if usage := poll(); usage.Calls != 1 || usage.Tokens != 120 {
		t.Fatalf("new local data not reflected: %+v", usage)
	}
	before, err := os.Stat(app.local.path)
	if err != nil {
		t.Fatal(err)
	}
	poll()
	after, err := os.Stat(app.local.path)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged poll rewrote disk cache", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if poll().Calls != 0 {
		t.Fatal("deleted session remained in live usage")
	}
	if upstreamCalls.Load() != 1 {
		t.Fatal("local polling forced remote quota refresh", upstreamCalls.Load())
	}
}
func TestAuthenticationAndForwarding(t *testing.T) {
	app := testApplication(t)
	handler := app.handler()
	request := func(method, path, host, origin, password, cookie string) *httptest.ResponseRecorder {
		t.Helper()
		body := ""
		if password != "" {
			body = `{"password":"` + password + `"}`
		}
		req := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
		req.Host = host
		req.Header.Set("Origin", origin)
		req.Header.Set("Cookie", cookie)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	for _, host := range []string{"localhost:54734", "127.0.0.1:54734", "[::1]:54734"} {
		if r := request("GET", "/", host, "", "", ""); r.Code != 200 {
			t.Fatal(host, r.Code, r.Body.String())
		}
	}
	for _, host := range []string{"evil.example:54734", "localhost.evil.example:54734", "localhost:65536", "localhost:0"} {
		if r := request("GET", "/", host, "", "", ""); r.Code != 403 {
			t.Fatal(host, r.Code)
		}
	}
	for _, path := range []string{"/api/local", "/api/account", "/api/limits", "/.state-codex-tally/sessions.json", "/pricing.json"} {
		if r := request("GET", path, "localhost:54734", "", "", ""); r.Code != 401 {
			t.Fatal(path, r.Code)
		}
	}
	if r := request("POST", "/api/login", "localhost:54734", "http://localhost:9999", "test-password-never-use-in-production", ""); r.Code != 403 {
		t.Fatal("cross origin login accepted")
	}
	login := request("POST", "/api/login", "localhost:54734", "http://localhost:54734", "test-password-never-use-in-production", "")
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure cookie")
	}
	local := request("GET", "/api/local", "localhost:54734", "http://localhost:54734", "", cookie.String())
	if local.Code != 200 {
		t.Fatal(local.Code, local.Body.String())
	}
	var usage localUsage
	if err := json.Unmarshal(local.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if !usage.CacheReady || usage.Files != 0 {
		t.Fatal("bad local usage")
	}
	if r := request("GET", "/api/local?period=custom7d&end=2026-02-31", "localhost:54734", "", "", cookie.String()); r.Code != 400 {
		t.Fatal("invalid period accepted")
	}
	if r := request("GET", "/.state-codex-tally/sessions.json", "localhost:54734", "", "", cookie.String()); r.Code != 404 {
		t.Fatal("private cache exposed")
	}
	request("POST", "/api/logout", "localhost:54734", "", "", cookie.String())
	if r := request("GET", "/api/local", "localhost:54734", "", "", cookie.String()); r.Code != 401 {
		t.Fatal("session survived logout")
	}
	// Reserve attempts before hashing: concurrent login failures are bounded as well.
	for range 10 {
		request("POST", "/api/login", "localhost:54734", "", "wrong", "")
	}
	if r := request("POST", "/api/login", "localhost:54734", "", "wrong", ""); r.Code != 429 {
		t.Fatal("login throttle missing")
	}
	app.config.Origin = "https://usage.example.com"
	if !app.validHost("usage.example.com") || app.validHost("localhost:54734") {
		t.Fatal("configured public host must remain fixed")
	}
	rr := httptest.NewRecorder()
	app.cookie(rr, "test", 30)
	if !rr.Result().Cookies()[0].Secure {
		t.Fatal("public cookie needs Secure")
	}
}
func TestAccountNormalizationAndIsolation(t *testing.T) {
	counts, err := normalizeCounts([]byte(`{"balance_unit":"credit","data":[{"date":"2026-09-10","totals":{"turns":0},"clients":[{"client_id":"CLI","turns":2}],"models":[{"model":"m","credits":0}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(counts)
	if !strings.Contains(string(raw), `"text_total_tokens":null`) || !strings.Contains(string(raw), `"turns":0`) {
		t.Fatal(string(raw))
	}
	breakdown, err := normalizeBreakdown([]byte(`{"units":"percent","data":[{"date":"2026-09-10","models":[{"model":"m","credits":0.001}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(breakdown)
	if !strings.Contains(string(raw), `"units":"percent"`) {
		t.Fatal(string(raw))
	}
	body := `{"email":"private@example.com","access_token":"secret","account_id":"private","plan_type":"pro","rate_limit":{"primary_window":{"used_percent":55,"limit_window_seconds":604800,"reset_at":123}}}`
	limits, err := normalizeLimits([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(limits)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "secret") {
		t.Fatal("sensitive fields escaped")
	}
	calls := 0
	fail := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer access-secret" || r.Header.Get("ChatGPT-Account-Id") == "" {
			t.Error("missing backend authentication")
		}
		if fail {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer upstream.Close()
	client := newAccountClient(t.Context(), t.TempDir())
	client.base = upstream.URL + "/"
	first := client.read(credential{"access-secret", "account-a"}, "usage", nil, normalizeLimits, false)
	if first.Error != nil || first.Data == nil {
		t.Fatal("initial account read failed")
	}
	fail = true
	other := client.read(credential{"access-secret", "account-b"}, "usage", nil, normalizeLimits, false)
	if other.Data != nil || calls != 2 {
		t.Fatal("cache crossed accounts")
	}
}
func TestPasswordIgnoresLegacyFile(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "password")
	if err := os.WriteFile(path, []byte("existing-dashboard-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(t.Context(), config{Host: "127.0.0.1", Port: "4318", Root: t.TempDir(), Home: t.TempDir(), State: state}, "current-dashboard-password")
	if err != nil {
		t.Fatal(err)
	}
	for password, status := range map[string]int{"existing-dashboard-password": 401, "current-dashboard-password": 200} {
		req := httptest.NewRequest("POST", "http://localhost:4318/api/login", strings.NewReader(`{"password":"`+password+`"}`))
		rr := httptest.NewRecorder()
		app.handler().ServeHTTP(rr, req)
		if rr.Code != status {
			t.Fatalf("login status: got %d, want %d", rr.Code, status)
		}
	}
}
