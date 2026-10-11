package xiaohongshu

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStressContractsPrimaryCommentLimit(t *testing.T) {
	for _, tc := range []struct {
		loaded, limit, want int
		truncated           bool
	}{{10, 1, 1, true}, {19, 12, 12, true}, {10, 20, 10, false}} {
		t.Run(fmt.Sprintf("%d/%d", tc.loaded, tc.limit), func(t *testing.T) {
			c := &CommentList{Cursor: "server-next", List: make([]Comment, tc.loaded)}
			c.List[0].SubComments = []Comment{{ID: "reply1"}, {ID: "reply2"}}
			limitPrimaryComments(c, tc.limit)
			require.Len(t, c.List, tc.want)
			require.Len(t, c.List[0].SubComments, 2)
			require.Equal(t, tc.truncated, c.Truncated)
			if tc.truncated {
				require.Empty(t, c.Cursor)
				require.True(t, c.HasMore)
				require.Equal(t, tc.loaded, c.LoadedCount)
			} else {
				require.Equal(t, "server-next", c.Cursor)
			}
		})
	}
}
func TestStressContractsRejectInvalidInputWithoutPage(t *testing.T) {
	_, err := (&SearchAction{}).Search(context.Background(), " \t\n")
	require.ErrorContains(t, err, "[invalid_argument]")
	for _, speed := range []string{"", "slow", "normal", "fast"} {
		require.NoError(t, (CommentLoadConfig{ScrollSpeed: speed}).Validate())
	}
	_, err = (&FeedDetailAction{}).GetFeedDetailWithConfig(context.Background(), "fixture", "token", true, CommentLoadConfig{ScrollSpeed: "warp"})
	require.ErrorContains(t, err, "[invalid_argument]")
}

func TestStressContractsAuthenticationDoesNotTrustGuestIDsOrSidebar(t *testing.T) {
	b := searchFixtureBrowser(t)
	page := b.MustPage()
	defer page.Close()
	for _, tc := range []struct {
		name, info    string
		authenticated bool
	}{{"guest", "{guest:true,userId:'guest-id'}", false}, {"member", "{guest:false,userId:'member-id'}", true}} {
		t.Run(tc.name, func(t *testing.T) {
			page.MustEval(`(info)=>{document.body.innerHTML='<div class="main-container"><div class="user"><a class="link-wrapper"><span class="channel">我</span></a></div></div>';window.__INITIAL_STATE__={user:{userInfo:{_value:eval('('+info+')')}}}}`, tc.info)
			yes, err := waitAuthenticated(page, time.Second)
			require.NoError(t, err)
			require.Equal(t, tc.authenticated, yes)
		})
	}
	page.MustEval(`()=>{document.body.innerHTML='';window.__INITIAL_STATE__={}}`)
	_, err := waitAuthenticated(page, 150*time.Millisecond)
	require.ErrorContains(t, err, "[auth_state_not_ready]")
}

func TestStressContractsGuestUnreadIsErrorButAuthenticatedZeroIsValid(t *testing.T) {
	b := searchFixtureBrowser(t)
	for _, guest := range []bool{true, false} {
		t.Run(fmt.Sprint(guest), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `<html><script>window.__INITIAL_STATE__={user:{userInfo:{_value:{guest:%v,userId:'fixture'}}},notification:{notificationCount:{_value:{mentions:0,likes:0,connections:0,unreadCount:0}}}};</script></html>`, guest)
			}))
			defer server.Close()
			page := b.MustPage()
			defer page.Close()
			count, err := NewNotificationAction(page).unreadCountAtURL(context.Background(), server.URL)
			if guest {
				require.ErrorContains(t, err, "[login_required]")
				require.Nil(t, count)
			} else {
				require.NoError(t, err)
				require.Zero(t, count.Unread)
			}
		})
	}
}

func TestStressContractsSelfProfileUsesVerifiedHrefInsteadOfHiddenClick(t *testing.T) {
	b := searchFixtureBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/explore" {
			fmt.Fprint(w, `<html><div class="main-container"><li class="user side-bar-component" style="display:none"><a class="link-wrapper" href="/user/profile/fixture"><span class="channel">我</span></a></li></div><script>window.__INITIAL_STATE__={user:{userInfo:{_value:{guest:false,userId:'fixture'}}}};</script></html>`)
		} else {
			fmt.Fprint(w, `<html><script>window.__INITIAL_STATE__={user:{userInfo:{_value:{guest:false,userId:'fixture'}},userPageData:{_value:{basicInfo:{nickname:'fixture'},interactions:[]}},notes:{_value:[[{id:'profile-note'}]]},activeTab:{_value:{query:'note',index:0}}}};</script></html>`)
		}
	}))
	defer server.Close()
	page := b.MustPage()
	defer page.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, NewNavigate(page).toProfileFromHome(ctx, server.URL+"/explore"))
	profile, err := NewUserProfileAction(page).extractUserProfileData(page, TabNotes)
	require.NoError(t, err)
	require.Equal(t, "profile-note", profile.Feeds[0].ID)
}

func TestStressContractsProfileFailureIsBoundedAndClassified(t *testing.T) {
	b := searchFixtureBrowser(t)
	page := b.MustPage()
	defer page.Close()
	page.MustEval(`()=>{document.body.innerHTML='<div class="access-wrapper">unavailable</div>'}`)
	require.ErrorContains(t, waitProfileDataWithin(page, time.Second), "[profile_access_denied]")
	page.MustEval(`()=>{document.body.innerHTML='';window.__INITIAL_STATE__={}}`)
	started := time.Now()
	require.ErrorContains(t, waitProfileDataWithin(page, 150*time.Millisecond), "[profile_not_ready]")
	require.Less(t, time.Since(started), time.Second)
	page.MustEval(`()=>{document.body.innerHTML='<div class="reds-tab-item sub-tab-list">笔记</div><div class="reds-tab-item sub-tab-list" style="display:none">点赞</div>';window.__INITIAL_STATE__={user:{activeTab:{_value:{query:'note',index:0}}}}}`)
	require.ErrorContains(t, NewUserProfileAction(page).selectTab(context.Background(), page, TabLiked), "[profile_tab_unavailable]")
}

func TestStressContractsProfileAcceptsRawAndHydratedState(t *testing.T) {
	b := searchFixtureBrowser(t)
	page := b.MustPage()
	defer page.Close()
	for _, wrapper := range []string{"raw", "value", "_value"} {
		t.Run(wrapper, func(t *testing.T) {
			page.MustEval(`(wrapper)=>{const wrap=v=>wrapper==='raw'?v:{[wrapper]:v};window.__INITIAL_STATE__={user:{userPageData:wrap({basicInfo:{nickname:'fixture'},interactions:[]}),notes:wrap([[{id:'fixture-note'}]]),activeTab:wrap({query:'note',index:0})}}}`, wrapper)
			profile, err := NewUserProfileAction(page).extractUserProfileData(page, TabNotes)
			require.NoError(t, err)
			require.Len(t, profile.Feeds, 1)
			require.Equal(t, "fixture-note", profile.Feeds[0].ID)
		})
	}
}

func TestStressContractsProfileWaitsForDelayedTabControls(t *testing.T) {
	b := searchFixtureBrowser(t)
	page := b.MustPage()
	defer page.Close()
	page.MustEval(`()=>{document.body.innerHTML='';window.__INITIAL_STATE__={user:{activeTab:{query:'note',index:0}}};setTimeout(()=>{const el=document.createElement('div');el.className='reds-tab-item sub-tab-list';el.textContent='收藏';el.onclick=()=>{window.__INITIAL_STATE__.user.activeTab={query:'fav',index:1}};document.body.appendChild(el)},3500)}`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, NewUserProfileAction(page).selectTab(ctx, page.Context(ctx), TabFavorites))
}
