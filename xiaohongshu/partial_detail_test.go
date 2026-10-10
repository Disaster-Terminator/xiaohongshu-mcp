package xiaohongshu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Model the incident: authenticated SSR note data exists while a deferred
// dependency never returns, so the comment container and requests never exist.
func TestInitialCommentsReadinessPartialDetail(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required")
	}
	l := launcher.New().Bin(bin).Headless(true).Set("no-sandbox")
	b := rod.New().ControlURL(l.MustLaunch()).MustConnect()
	defer l.Cleanup()
	defer l.Kill()
	defer b.MustClose()
	cancelComments := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cancel" {
			select {
			case cancelComments <- struct{}{}:
			default:
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/pending" {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `<html><head><script defer src="/pending"></script></head><body><div class="access-wrapper"></div><script>
 window.__INITIAL_STATE__={note:{noteDetailMap:{fixture:{note:{noteId:'fixture',title:'available note',interactInfo:{commentCount:'9'}},comments:{list:[],cursor:'',hasMore:true}}}}};
 if(location.search.includes('security'))document.title='Security Verification';
 if(location.search.includes('cancel'))Object.defineProperty(window.__INITIAL_STATE__.note.noteDetailMap.fixture.comments,'list',{get(){fetch('/cancel');return []}});
 </script></body></html>`)
	}))
	defer server.Close()
	for _, scenario := range []string{"partial", "strict", "security", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			p := b.MustPage()
			defer p.Close()
			a := NewFeedDetailAction(p)
			a.initialCommentsTimeout = 100 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			target := server.URL + "/explore/fixture"
			if scenario == "security" {
				target += "?security"
			}
			if scenario == "cancel" {
				target += "?cancel"
				a.initialCommentsTimeout = 2 * time.Second
				go func() {
					select {
					case <-cancelComments:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			result, err := a.getFeedDetailAtURL(ctx, "fixture", target, false, DefaultCommentLoadConfig(), scenario == "strict")
			if scenario != "partial" {
				want := context.DeadlineExceeded
				if scenario == "cancel" {
					want = context.Canceled
				}
				if result != nil || !errors.Is(err, want) {
					t.Fatalf("must remain an error, result=%v error=%v", result, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !result.Partial || result.Note.Title != "available note" || result.Comments != nil || result.CommentsStatus != "unavailable" {
				t.Fatalf("lost note or fabricated comments: %+v", result)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"comments":null`) || strings.Contains(string(raw), `"list":[]`) {
				t.Fatalf("unloaded comments must be null: %s", raw)
			}
		})
	}
}
