package xiaohongshu

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func searchFixtureBrowser(t *testing.T) *rod.Browser {
	t.Helper()
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for browser regression")
	}
	l := launcher.New().Bin(bin).Headless(true).Set("no-sandbox")
	b := rod.New().ControlURL(l.MustLaunch()).MustConnect()
	t.Cleanup(func() { b.MustClose(); l.Kill(); l.Cleanup() })
	return b
}

func TestSearchNavigationUsesSPAAndWaitsForCurrentResults(t *testing.T) {
	b := searchFixtureBrowser(t)
	for _, empty := range []bool{false, true} {
		for _, doubleEncoded := range []bool{false, true} {
			t.Run(fmt.Sprintf("empty=%v/double_encoded=%v", empty, doubleEncoded), func(t *testing.T) {
				var directLoads atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/search_result_ai" {
						directLoads.Add(1)
						http.Redirect(w, r, "/website-login/captcha", http.StatusFound)
						return
					}
					fmt.Fprintf(w, `<html><body>
<div id="app">
<input id="search-input" style="display:none"><button class="search-icon" style="display:none">search</button>
<div class="search-empty" style="display:none">暂无相关结果</div>
<div class="filter" style="display:none">old hidden filter</div>
<div class="filter ai-chat-filter">AI filter</div>
<script>
window.__INITIAL_STATE__={user:{userInfo:{value:{guest:false,userId:'fixture'}}},search:{feeds:{value:[{id:'stale'}]}}};
setTimeout(()=>{
document.querySelector('.search-icon').onclick=()=>{
 window.submitted=document.querySelector('#search-input').value;
 history.pushState({},'', '/search_result_ai?keyword='+encodeURIComponent(%s));
 setTimeout(()=>{
   window.__INITIAL_STATE__.search.feeds={_value:%s};
   if(%v) document.querySelector('.search-empty').style.display='block';
 },350);
};
document.querySelector('#app').__vue_app__={};
},150);
</script></div></body></html>`, map[bool]string{false: `window.submitted`, true: `encodeURIComponent(window.submitted)`}[doubleEncoded], map[bool]string{false: `[{id:'current'}]`, true: `[]`}[empty], empty)
				}))
				defer server.Close()
				page := b.MustPage().Timeout(5 * time.Second)
				defer page.Close()
				keyword := `咖啡 & "fixture"`
				start := time.Now()
				require.NoError(t, navigateSearchFromHome(page, keyword, server.URL+"/explore"))
				require.GreaterOrEqual(t, time.Since(start), 450*time.Millisecond)
				require.Zero(t, directLoads.Load(), "search must not load the protected direct URL")
				require.Equal(t, keyword, page.MustEval(`() => window.submitted`).String())
				want := "current"
				if empty {
					want = ""
				}
				require.Equal(t, want, readFeedIDs(page))
				filter, err := visibleSearchFilter(page)
				require.NoError(t, err)
				require.Equal(t, "AI filter", filter.MustText())
			})
		}
	}
}

func TestSearchNavigationReturnsSecurityVerificationWithoutWaitingForDeadline(t *testing.T) {
	b := searchFixtureBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/explore" {
			http.Redirect(w, r, "/website-login/captcha", http.StatusFound)
			return
		}
		fmt.Fprint(w, `<html><title>Security Verification</title><body>Scan to verify</body></html>`)
	}))
	defer server.Close()
	page := b.MustPage().Timeout(5 * time.Second)
	defer page.Close()
	start := time.Now()
	err := navigateSearchFromHome(page, "fixture", server.URL+"/explore")
	require.ErrorContains(t, err, "[security_verification]")
	require.Less(t, time.Since(start), 3*time.Second)
}

func TestSearchNavigationRejectsStaleDataAndHonorsCancellation(t *testing.T) {
	b := searchFixtureBrowser(t)
	page := b.MustPage().Timeout(5 * time.Second)
	defer page.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>window.__INITIAL_STATE__={search:{feeds:{value:[{id:'stale'}]}}};</script>`)
	}))
	defer server.Close()
	page.MustNavigate(server.URL + "/search_result_ai?keyword=fixture")
	page.MustWait(`() => !!window.__INITIAL_STATE__`)
	err := waitSearchPage(page, "fixture", "results", "stale", 200*time.Millisecond)
	require.ErrorContains(t, err, "[search_page_not_ready]")
	page.MustEval(`() => {
		history.replaceState({}, '', '/search_result_ai?keyword=another-query');
		window.__INITIAL_STATE__.search.feeds = {value:[{id:'another-result'}]};
	}`)
	err = waitSearchPage(page, "fixture", "results", "stale", 200*time.Millisecond)
	require.ErrorContains(t, err, "[search_page_not_ready]", "fresh results for a different query must be rejected")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = waitSearchPage(page.Context(ctx), "fixture", "results", "stale", time.Second)
	require.True(t, errors.Is(err, context.Canceled))
}
