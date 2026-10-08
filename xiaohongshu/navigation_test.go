package xiaohongshu

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestDocumentReadinessDoesNotWaitForPendingImagesOrReturnOldData(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for browser regression")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pending" {
			<-r.Context().Done()
			return
		}
		fmt.Fprintf(w, `<html><body><img src="/pending"><script>
window.__INITIAL_STATE__={search:{feeds:{value:[%q]}}};
</script></body></html>`, r.URL.Path)
	}))
	defer server.Close()
	l := launcher.New().Bin(bin).Headless(true).Set("no-sandbox")
	b := rod.New().ControlURL(l.MustLaunch()).MustConnect()
	defer l.Cleanup()
	defer l.Kill()
	defer b.MustClose()
	page := b.MustPage().Timeout(5 * time.Second)
	defer page.Close()
	for _, path := range []string{"/old", "/new"} {
		start := time.Now()
		navigateDocument(page, server.URL+path)
		got := page.MustEval(`() => window.__INITIAL_STATE__.search.feeds.value[0]`).String()
		if got != path {
			t.Fatalf("returned stale data: got %q, want %q", got, path)
		}
		if time.Since(start) > 3*time.Second {
			t.Fatal("waited for pending asset instead of document readiness")
		}
	}
}
