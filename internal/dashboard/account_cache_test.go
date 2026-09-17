package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountCacheExpiryAndRetry(t *testing.T) {
	var requests atomic.Int64
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"plan_type":"pro","rate_limit":null}`)
	}))
	defer upstream.Close()
	now := time.Now()
	client := newAccountClient(t.Context(), t.TempDir())
	client.now = func() time.Time { return now }
	client.base = upstream.URL + "/"
	read := func(refresh bool) remoteResult {
		return client.read(credential{"access", "account"}, "usage", nil, normalizeLimits, refresh)
	}
	first := read(false)
	if first.Error != nil || first.RefreshAfterMS != (30*time.Minute).Milliseconds() {
		t.Fatal("wrong success lifetime", first)
	}
	now = now.Add(29 * time.Minute)
	cached := read(false)
	if requests.Load() != 1 || !cached.Cached || cached.RefreshAfterMS != time.Minute.Milliseconds() {
		t.Fatal("cache hit extended deadline", cached)
	}
	now = now.Add(time.Minute)
	fail.Store(true)
	stale := read(false)
	if requests.Load() != 2 || stale.Error == nil || stale.Data == nil || !stale.FetchedAt.Equal(*first.FetchedAt) || stale.RefreshAfterMS != (30*time.Second).Milliseconds() {
		t.Fatal("failed refresh lost data or retry deadline", stale)
	}
	now = now.Add(29 * time.Second)
	if r := read(false); requests.Load() != 2 || r.RefreshAfterMS != 1000 {
		t.Fatal("retried too early", r)
	}
	now = now.Add(time.Second)
	fail.Store(false)
	recovered := read(false)
	if requests.Load() != 3 || recovered.Error != nil || recovered.Cached || !recovered.FetchedAt.Equal(now) || recovered.RefreshAfterMS != (30*time.Minute).Milliseconds() {
		t.Fatal("did not recover", recovered)
	}
	read(true)
	if requests.Load() != 4 {
		t.Fatal("manual refresh did not bypass cache")
	}
}

func TestAccountColdFailureRetries(t *testing.T) {
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"plan_type":"plus","rate_limit":null}`)
	}))
	defer upstream.Close()
	now := time.Now()
	client := newAccountClient(t.Context(), t.TempDir())
	client.now = func() time.Time { return now }
	client.base = upstream.URL + "/"
	read := func() remoteResult {
		return client.read(credential{"access", "account"}, "usage", nil, normalizeLimits, false)
	}
	first := read()
	if first.Error == nil || first.Data != nil {
		t.Fatal("expected empty failure")
	}
	read()
	if requests.Load() != 1 {
		t.Fatal("failure cache ignored")
	}
	now = now.Add(30 * time.Second)
	if next := read(); next.Error != nil || next.Data == nil || requests.Load() != 2 {
		t.Fatal("cold failure never recovered", next)
	}
}
