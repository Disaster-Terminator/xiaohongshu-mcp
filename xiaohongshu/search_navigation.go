package xiaohongshu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const searchHomeURL = "https://www.xiaohongshu.com/explore"

// Search is an in-site SPA transition. A committed-document wait belongs only
// to the initial explore navigation, never to clicking the search control.
func navigateSearch(page *rod.Page, keyword string) error {
	return navigateSearchFromHome(page, keyword, searchHomeURL)
}

func navigateSearchFromHome(page *rod.Page, keyword, homeURL string) error {
	if err := navigateDocumentChecked(page.Timeout(20*time.Second), homeURL); err != nil {
		return fmt.Errorf("[search_navigation] 搜索主页导航失败: %w", err)
	}
	// Cold homepage initialization can exceed 20s even when its security
	// dependency completes normally. The outer search context still caps
	// the entire operation at 60s, including the results phase.
	if err := waitSearchPage(page, keyword, "home", "", 30*time.Second); err != nil {
		return err
	}
	before := readFeedIDs(page)
	result, err := page.Eval(submitSearchJS, keyword)
	if err != nil {
		return fmt.Errorf("[search_navigation] 提交页面搜索失败: %w", err)
	}
	if result.Value.String() != "submitted" {
		return fmt.Errorf("[search_navigation] 页面搜索控件不可用")
	}
	return waitSearchPage(page, keyword, "results", before, 20*time.Second)
}

const submitSearchJS = `(keyword) => {
	const input = document.querySelector('#search-input');
	const icon = document.querySelector('.search-icon');
	if (!input || !icon) return 'missing';
	const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
	setter.call(input, keyword);
	input.dispatchEvent(new Event('input', {bubbles: true}));
	icon.click();
	return 'submitted';
}`

const searchPageStateJS = `(keyword, phase, before) => {
	const visible = el => !!el && el.getClientRects().length > 0;
	if (location.pathname.startsWith('/website-login/') ||
		/^(Security Verification|安全验证)$/i.test(document.title.trim()) ||
		visible(document.querySelector('.r-captcha-modal'))) return 'security_verification';
	const state = window.__INITIAL_STATE__ || {};
	const userRef = state.user?.userInfo;
	const user = userRef?.value ?? userRef?._value ?? userRef;
	const login = visible(document.querySelector('.login-container'));
	if (phase === 'home') {
		const hydrated = user && (user.userId || user.user_id || user.guest === true);
		// The new homepage retains hidden classic search controls. Use site
		// data and the resulting query transition, not framework internals.
		if (hydrated && !login && document.querySelector('#search-input') &&
			document.querySelector('.search-icon')) return 'ready';
		return login ? 'login_required' : 'pending';
	}
	const url = new URL(location.href);
	// The site's SPA currently encodes keyword before giving it to the router,
	// so URLSearchParams can still contain one encoded layer. Decode only that
	// observed layer, and still require the exact original query.
	const query = url.searchParams.get('keyword');
	let queryMatches = query === keyword;
	if (!queryMatches && query !== null) {
		try { queryMatches = decodeURIComponent(query) === keyword; } catch (_) {}
	}
	if (!url.pathname.startsWith('/search_result') || !queryMatches)
		return login ? 'login_required' : 'pending';
	const store = state.search?.feeds;
	const feeds = store?.value ?? store?._value ?? store;
	if (!Array.isArray(feeds)) return 'pending';
	if (feeds.length && feeds.map(feed => feed.id).join(',') !== before) return 'ready';
	const empty = [...document.querySelectorAll('.no-result, .search-empty, .empty-container, .empty-page')]
		.some(el => visible(el) && /没有找到|暂无|没有相关|无结果|no results/i.test(el.innerText));
	return !feeds.length && empty ? 'ready' : 'pending';
}`

func waitSearchPage(page *rod.Page, keyword, phase, before string, timeout time.Duration) error {
	return waitSearchPageWithControls(page, keyword, phase, before, timeout, searchControlsBound)
}

func waitSearchPageWithControls(page *rod.Page, keyword, phase, before string, timeout time.Duration, controlsBound func(*rod.Page) (bool, error)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	lastState := "pending"
	for {
		result, err := page.Eval(searchPageStateJS, keyword, phase, before)
		if err != nil {
			return fmt.Errorf("[search_page_state] 无法读取搜索页面状态: %w", err)
		}
		lastState = result.Value.String()
		switch lastState {
		case "ready":
			if phase != "home" {
				return nil
			}
			bound, err := controlsBound(page)
			if err != nil {
				if requestErr := page.GetContext().Err(); requestErr != nil {
					return requestErr
				}
				// A busy renderer can exhaust this observation's 2s context
				// while the search still has time left. Keep waiting under
				// the original phase/request budgets; never retry submission.
				if !errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logrus.WithField("phase", phase).Warn("Search control observation timed out; waiting within phase budget")
			}
			if bound {
				return nil
			}
			lastState = "controls_not_bound"
		case "security_verification":
			return fmt.Errorf("[security_verification] 小红书页面要求安全验证；本次搜索已停止，未自动重试")
		}
		select {
		case <-page.GetContext().Done():
			return page.GetContext().Err()
		case <-deadline.C:
			if diagnostic, diagnosticErr := page.Eval(`(keyword) => {
				const state = window.__INITIAL_STATE__ || {};
				const ref = state.user?.userInfo, user = ref?.value ?? ref?._value ?? ref;
				const input = document.querySelector('#search-input'), icon = document.querySelector('.search-icon');
				const store = state.search?.feeds, feeds = store?.value ?? store?._value ?? store;
				const query = new URL(location.href).searchParams.get('keyword');
				let decodedMatches = false;
				try { decodedMatches = query !== null && decodeURIComponent(query) === keyword; } catch (_) {}
				return JSON.stringify({userHydrated:!!(user?.userId || user?.user_id),
					inputPresent:!!input, iconPresent:!!icon,
					searchRoute:location.pathname.startsWith('/search_result'), queryOnceMatches:query === keyword, queryTwiceMatches:decodedMatches,
					feedCount:Array.isArray(feeds)?feeds.length:null});
			}`, keyword); diagnosticErr == nil {
				logrus.WithFields(logrus.Fields{"phase": phase, "page_state": lastState, "signals": diagnostic.Value.String()}).Warn("Search page not ready")
			}
			if lastState == "login_required" {
				return fmt.Errorf("[login_required] 小红书页面要求登录")
			}
			return fmt.Errorf("[search_page_not_ready] 搜索页面或当前查询的数据未在限定时间内就绪 (phase=%s)", phase)
		case <-ticker.C:
		}
	}
}

// Server-rendered controls can exist before their input/click handlers bind.
// CDP exposes standard DOM listeners without depending on Vue internals. Only
// listener presence is inspected; handler source or account data is not read.
func searchControlsBound(page *rod.Page) (bool, error) {
	result, err := (proto.RuntimeEvaluate{
		Expression: `(() => {
			const bound = (node, type) => {
				for (let depth = 0; node && depth < 4; depth++, node = node.parentElement) {
					if ((getEventListeners(node)[type] || []).length) return true;
				}
				return false;
			};
			return {inputBound:bound(document.querySelector('#search-input'), 'input'),
				clickBound:bound(document.querySelector('.search-icon'), 'click')};
		})()`,
		IncludeCommandLineAPI: true,
		ReturnByValue:         true,
	}).Call(page.Timeout(2 * time.Second))
	if err != nil {
		return false, fmt.Errorf("[search_page_state] 无法读取搜索控件事件状态: %w", err)
	}
	if result.ExceptionDetails != nil || result.Result == nil {
		return false, fmt.Errorf("[search_page_state] 搜索控件事件状态不可用")
	}
	return result.Result.Value.Get("inputBound").Bool() && result.Result.Value.Get("clickBound").Bool(), nil
}

func visibleSearchFilter(page *rod.Page) (*rod.Element, error) {
	elements, err := page.Elements(".filter.ai-chat-filter, .search-layout__top .filter, div.filter")
	if err != nil {
		return nil, err
	}
	for _, element := range elements {
		if humanize.Visible(element) {
			return element, nil
		}
	}
	return nil, fmt.Errorf("[search_filter_missing] 当前搜索页面没有可见的筛选入口")
}
