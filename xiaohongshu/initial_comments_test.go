package xiaohongshu

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestInitialCommentsReadiness(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for browser regression")
	}
	l := launcher.New().Bin(bin).Headless(true).Set("no-sandbox")
	b := rod.New().ControlURL(l.MustLaunch()).MustConnect()
	defer l.Cleanup()
	defer l.Kill()
	defer b.MustClose()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><div style="height:1200px"></div><div class="comments-container">comments</div><script>
window.scrolls=0;
window.__INITIAL_STATE__={note:{noteDetailMap:{fixture:{note:{interactInfo:{commentCount:"147"}},comments:{list:[],cursor:"",hasMore:true}}}}};
window.addEventListener('scroll', () => {
 window.scrolls++;
 if (location.pathname === '/lazy') setTimeout(() => {
  const c=window.__INITIAL_STATE__.note.noteDetailMap.fixture.comments;
  c.list=[{id:'first'}]; c.cursor='next';
 },250);
});
</script></body></html>`)
	}))
	defer server.Close()
	for _, scenario := range []string{"lazy", "loaded", "empty", "pending"} {
		t.Run(scenario, func(t *testing.T) {
			page := b.MustPage().Timeout(5 * time.Second)
			defer page.Close()
			navigateDocument(page, server.URL+"/"+scenario)
			page.MustWait(`() => !!window.__INITIAL_STATE__`)
			switch scenario {
			case "loaded":
				page.MustEval(`() => { window.__INITIAL_STATE__.note.noteDetailMap.fixture.comments.list=[{id:'existing'}]; }`)
			case "empty":
				page.MustEval(`() => { window.__INITIAL_STATE__.note.noteDetailMap.fixture.note.interactInfo.commentCount="0"; }`)
			case "pending":
				page = page.Timeout(100 * time.Millisecond)
			}
			err := waitInitialComments(page, "fixture")
			if scenario == "pending" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("pending comments must fail, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "lazy" && page.MustEval(`() => window.__INITIAL_STATE__.note.noteDetailMap.fixture.comments.list.length`).Int() != 1 {
				t.Fatal("returned before first comments loaded")
			}
			if scenario != "lazy" && page.MustEval(`() => window.scrolls`).Int() != 0 {
				t.Fatal("already loaded or genuinely empty comments must not trigger scrolling")
			}
		})
	}
}
