package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExpiredQuotaRevalidatesOnce(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("upstream_failure=", fail), func(t *testing.T) {
			root := t.TempDir()
			if err := atomicJSON(filepath.Join(root, "auth.json"), map[string]any{"tokens": credential{"test-access", "test-account"}}); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			reset := time.Now().Add(24 * time.Hour).Unix()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				if count > 1 && fail {
					w.WriteHeader(503)
					return
				}
				end := reset
				if count == 1 {
					end = time.Now().Add(-time.Hour).Unix()
				}
				fmt.Fprintf(w, `{"plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_at":%d}}}`, end)
			}))
			defer upstream.Close()
			client := newAccountClient(t.Context(), root)
			client.base = upstream.URL + "/"
			client.read(credential{"test-access", "test-account"}, "usage", nil, normalizeLimits, false)
			window, err := client.quotaPeriod(604800, false)
			if calls.Load() != 2 {
				t.Fatalf("expected one revalidation; requests=%d", calls.Load())
			}
			if fail {
				if err == nil || !strings.Contains(err.Error(), "503") || window != (period{}) {
					t.Fatalf("expired data must not be used: %v", err)
				}
				client.quotaPeriod(604800, false)
				if calls.Load() != 2 {
					t.Fatal("expired quota bypassed failure retry delay")
				}
				return
			}
			if err != nil || window.To != reset*1000 || window.From != (reset-604800)*1000 {
				t.Fatalf("wrong refreshed window: %+v %v", window, err)
			}
			if _, err := client.quotaPeriod(604800, false); err != nil || calls.Load() != 2 {
				t.Fatal("valid window was fetched again")
			}
		})
	}
}
