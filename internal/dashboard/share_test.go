package dashboard

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func publicFixture() *snapshot {
	date := time.Date(2026, 9, 16, 12, 0, 0, 0, time.Local)
	return &snapshot{FetchedAt: date, Source: "private-source", Unreadable: 12345, Files: map[string]session{
		"private-project/session.jsonl": {Events: []event{
			{Timestamp: date.Add(-time.Hour).UnixMilli(), Model: "test-model", Input: 20, Cached: 60, Output: 20, Tokens: 100},
			{Timestamp: date.Add(-2 * time.Hour).UnixMilli(), Model: `<script>alert("model")</script>`, Input: 20, Tokens: 20},
			{Timestamp: date.AddDate(0, -1, 0).UnixMilli(), Model: "previous-month-private", Tokens: 9000},
			{Timestamp: date.Add(time.Hour).UnixMilli(), Model: "future-private", Tokens: 8000},
		}},
	}}
}

func TestPublicComponentsBoundary(t *testing.T) {
	app := testApplication(t)
	handler := app.handler()
	get := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost:4318"+path, nil)
		r.Header.Set("Origin", "https://blog.example")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/share", "/share/tokens.svg"} {
		if got := get("GET", path).Code; got != 404 {
			t.Fatalf("disabled %s: %d", path, got)
		}
	}
	app.config.Share = true
	app.local.public = new(publicSnapshot)
	if got := get("GET", "/share"); got.Code != 503 || got.Header().Get("Retry-After") == "" {
		t.Fatal("missing snapshot must not trigger a scan")
	}
	if err := app.local.public.publish(publicFixture()); err != nil {
		t.Fatal(err)
	}
	for component := range publicComponents {
		for _, ext := range []string{"", ".svg"} {
			for _, theme := range []string{"auto", "light", "dark"} {
				path := "/share/" + component + ext + "?theme=" + theme
				w := get("GET", path)
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
				body := w.Body.String()
				for _, forbidden := range []string{"private-source", "private-project", "previous-month-private", "future-private", "费用", "额度", "活跃", "auth.json", "access_token", "<script>alert"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("%s leaks %q", path, forbidden)
					}
				}
				if w.Header().Get("X-Frame-Options") != "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors *") {
					t.Fatal("public embedding blocked")
				}
				if w.Header().Get("Set-Cookie") != "" {
					t.Fatal("public request creates a session")
				}
				csp := w.Header().Get("Content-Security-Policy")
				if ext == "" {
					if strings.Count(body, "<script>") != 1 || !strings.Contains(body, "<script>"+resizeScript+"</script>") || !strings.Contains(csp, "script-src "+resizeHash) || !strings.Contains(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
						t.Fatal("only the immutable resize script may run inside the opaque sandbox")
					}
				} else if strings.Contains(body, "<script") || strings.Contains(csp, "allow-scripts") {
					t.Fatal("SVG must remain script-free")
				}
				if ext == ".svg" {
					if w.Header().Get("Content-Type") != "image/svg+xml" {
						t.Fatal("wrong image type")
					}
					decoder := xml.NewDecoder(strings.NewReader(body))
					for {
						_, err := decoder.Token()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Fatalf("invalid SVG: %v", err)
						}
					}
				}
			}
		}
	}
	for path, expected := range map[string]string{"/share/tokens": "120", "/share/calls": "2", "/share/cache": "60.0%"} {
		if !strings.Contains(get("GET", path).Body.String(), expected) {
			t.Fatalf("incorrect aggregate: %s", path)
		}
	}
	before := get("GET", "/share").Body.String()
	if head := get("HEAD", "/share/tokens.svg"); head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatal("public image HEAD contract")
	}
	if got := get("GET", "/share?refresh=1&period=last_month").Body.String(); got != before {
		t.Fatal("visitor parameters changed snapshot")
	}
	if _, pending, _ := app.local.view(); pending {
		t.Fatal("public request started scanning")
	}
	for _, path := range []string{"/api/local?refresh=1", "/api/account?refresh=1", "/.state-codex-tally/sessions.json", "/share.html", "/share.svg"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:4318"+path, nil))
		if w.Code != 401 {
			t.Fatalf("private endpoint %s: %d", path, w.Code)
		}
	}
	if get("POST", "/share").Code == 200 {
		t.Fatal("public writes allowed")
	}
	for _, path := range []string{"/share/../api/local", "/share/%2e%2e/api/local", "/share/%2e%2e%2fapi%2flocal"} {
		if get("GET", path).Code == 200 {
			t.Fatalf("public path escaped into private handler: %s", path)
		}
	}
	if get("GET", "/share/tokens?theme=invalid").Code != 400 || get("GET", "/share/private.svg").Code != 404 {
		t.Fatal("unbounded public variant")
	}
	request := httptest.NewRequest("GET", "http://evil.example/share/tokens.svg", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != 403 {
		t.Fatal("host boundary removed")
	}
	app.config.Share = false
	if get("GET", "/share/tokens.svg").Code != 404 {
		t.Fatal("disabled share still accessible")
	}
}

func TestPublicPublishAfterRefresh(t *testing.T) {
	app := testApplication(t)
	app.config.Share = true
	app.local.public = new(publicSnapshot)
	<-app.local.refresh()
	w := httptest.NewRecorder()
	app.handler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:4318/share/tokens.svg", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ">0</text>") {
		t.Fatalf("scan did not publish: %d", w.Code)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 10 {
			if err := app.local.public.publish(publicFixture()); err != nil {
				t.Error(err)
			}
		}
	})
	for range 10 {
		w := httptest.NewRecorder()
		app.handler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:4318/share", nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Error("partial snapshot observed")
		}
	}
	wg.Wait()
}

func TestPublicHubCombinations(t *testing.T) {
	app := testApplication(t)
	app.config.Share = true
	app.local.public = new(publicSnapshot)
	if err := app.local.public.publish(publicFixture()); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		app.handler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:4318"+path, nil))
		return w
	}
	for mask := 1; mask < 16; mask++ {
		var selected []string
		for i, name := range hubComponents {
			if mask&(1<<i) != 0 {
				selected = append(selected, name)
			}
		}
		for _, ext := range []string{"", ".svg"} {
			w := get("/share/hub" + ext + "?components=" + strings.Join(selected, ","))
			if w.Code != 200 {
				t.Fatalf("combination %v: %d %s", selected, w.Code, w.Body.String())
			}
			body := w.Body.String()
			for i, name := range hubComponents {
				if strings.Contains(body, `data-component="`+name+`"`) != (mask&(1<<i) != 0) {
					t.Fatalf("wrong selected content: %v %s", selected, name)
				}
			}
			if mask&8 == 0 && strings.Contains(body, "test-model") {
				t.Fatal("unselected model data exposed")
			}
			for _, private := range []string{"private-source", "private-project", "previous-month-private", "future-private", "费用", "额度", "auth.json", "<script>alert"} {
				if strings.Contains(body, private) {
					t.Fatalf("hub leaks %s", private)
				}
			}
			if ext == ".svg" {
				decoder := xml.NewDecoder(strings.NewReader(body))
				for {
					_, err := decoder.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	for _, invalid := range []string{"", "tokens,tokens", "overview", "private", "tokens,auth.json", "tokens,", strings.Repeat("tokens,", 20)} {
		if w := get("/share/hub?components=" + invalid); w.Code != 400 {
			t.Fatalf("invalid components accepted: %q", invalid)
		}
	}
	if get("/share/hub?components=models,tokens").Body.String() != get("/share/hub?components=tokens,models").Body.String() {
		t.Fatal("equivalent selections must use the same snapshot")
	}
	if get("/share/hub").Body.String() != get("/share/hub?components=tokens,calls,cache,models").Body.String() {
		t.Fatal("default hub selection")
	}
	if get("/share/hub?components=tokens&refresh=1").Body.String() != get("/share/hub?components=tokens").Body.String() {
		t.Fatal("anonymous refresh changed hub")
	}
	app.config.Share = false
	if get("/share/hub").Code != 404 {
		t.Fatal("hub accessible while sharing disabled")
	}
}
