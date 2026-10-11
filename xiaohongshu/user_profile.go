package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// ProfileTab 个人主页的子 tab。
type ProfileTab string

const (
	TabNotes     ProfileTab = "note"
	TabFavorites ProfileTab = "fav"
	TabLiked     ProfileTab = "liked"
)

// ParseProfileTab 解析 tab 名，空值默认为「笔记」。
func ParseProfileTab(s string) (ProfileTab, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "note", "notes", "笔记":
		return TabNotes, nil
	case "fav", "favorites", "favorite", "收藏":
		return TabFavorites, nil
	case "liked", "like", "点赞":
		return TabLiked, nil
	}
	return "", fmt.Errorf("未知的主页 tab %q，可选：note / fav / liked", s)
}

// tabLabel 子 tab 对应的页面文字。
var tabLabel = map[ProfileTab]string{
	TabNotes:     "笔记",
	TabFavorites: "收藏",
	TabLiked:     "点赞",
}

type UserProfileAction struct {
	page *rod.Page
}

func NewUserProfileAction(page *rod.Page) *UserProfileAction {
	pp := page.Timeout(60 * time.Second)
	return &UserProfileAction{page: pp}
}

// UserProfile 获取用户基本信息及指定 tab 下的帖子
func (u *UserProfileAction) UserProfile(ctx context.Context, userID, xsecToken string, tab ProfileTab) (*UserProfileResponse, error) {
	page := u.page.Context(ctx).Timeout(60 * time.Second) // 重设被 .Context 清掉的 deadline

	searchURL := makeUserProfileURL(userID, xsecToken, tab)
	if err := navigateDocumentChecked(page.Timeout(15*time.Second), searchURL); err != nil {
		return nil, err
	}
	if err := waitProfileData(page); err != nil {
		return nil, err
	}
	if err := u.selectTab(ctx, page, tab); err != nil {
		return nil, err
	}
	return u.extractUserProfileData(page, tab)
}

// extractUserProfileData 从页面中提取用户资料数据的通用方法
func (u *UserProfileAction) extractUserProfileData(page *rod.Page, tab ProfileTab) (*UserProfileResponse, error) {
	if err := waitProfileData(page); err != nil {
		return nil, err
	}

	userDataResult := page.MustEval(`() => {
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.user &&
		    window.__INITIAL_STATE__.user.userPageData) {
			const userPageData = window.__INITIAL_STATE__.user.userPageData;
			const data = userPageData.value ?? userPageData._value ?? userPageData;
			if (data) {
				return JSON.stringify(data);
			}
		}
		return "";
	}`).String()

	if userDataResult == "" {
		return nil, fmt.Errorf("user.userPageData.value not found in __INITIAL_STATE__")
	}

	// 2. 获取用户帖子及当前 tab：window.__INITIAL_STATE__.user
	notesResult := page.MustEval(`() => {
		const u = window.__INITIAL_STATE__ && window.__INITIAL_STATE__.user;
		if (!u || !u.notes) return "";
		const unwrap = (o) => o?.value ?? o?._value ?? o;
		const notes = unwrap(u.notes);
		if (!notes) return "";
		const active = unwrap(u.activeTab) || {};
		return JSON.stringify({notes: notes, index: active.index || 0, query: active.query || ""});
	}`).String()

	if notesResult == "" {
		return nil, fmt.Errorf("user.notes.value not found in __INITIAL_STATE__")
	}

	// 解析用户信息
	var userPageData struct {
		Interactions []UserInteractions `json:"interactions"`
		BasicInfo    UserBasicInfo      `json:"basicInfo"`
	}
	if err := json.Unmarshal([]byte(userDataResult), &userPageData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal userPageData: %w", err)
	}

	var notesData struct {
		Notes [][]Feed `json:"notes"`
		Index int      `json:"index"`
		Query string   `json:"query"`
	}
	if err := json.Unmarshal([]byte(notesResult), &notesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notes: %w", err)
	}

	// tab 不符时报错，避免把别的 tab 的内容当成结果返回
	want := tab
	if want == "" {
		want = TabNotes
	}
	if notesData.Query != "" && ProfileTab(notesData.Query) != want {
		return nil, fmt.Errorf("[profile_tab_unavailable] 当前 tab 与请求的 tab 不符，不能返回其他 tab 内容")
	}

	// 组装响应
	response := &UserProfileResponse{
		UserBasicInfo: userPageData.BasicInfo,
		Interactions:  userPageData.Interactions,
	}

	// 每个 tab 的内容存在各自的下标里，只取当前 tab 的，避免混入其他 tab
	if notesData.Index >= 0 && notesData.Index < len(notesData.Notes) {
		response.Feeds = append(response.Feeds, notesData.Notes[notesData.Index]...)
	}

	return response, nil
}

func makeUserProfileURL(userID, xsecToken string, tab ProfileTab) string {
	url := fmt.Sprintf("https://www.xiaohongshu.com/user/profile/%s?xsec_token=%s&xsec_source=pc_note", userID, xsecToken)
	if tab != "" && tab != TabNotes {
		url += fmt.Sprintf("&tab=%s&subTab=note", tab)
	}
	return url
}

func (u *UserProfileAction) GetMyProfileViaSidebar(ctx context.Context, tab ProfileTab) (*UserProfileResponse, error) {
	page := u.page.Context(ctx).Timeout(60 * time.Second) // 重设被 .Context 清掉的 deadline

	// 创建导航动作
	navigate := NewNavigate(page)

	// 通过侧边栏导航到个人主页
	if err := navigate.ToProfilePage(ctx); err != nil {
		return nil, fmt.Errorf("failed to navigate to profile page via sidebar: %w", err)
	}

	// 等待页面加载完成并获取 __INITIAL_STATE__
	if err := waitProfileData(page); err != nil {
		return nil, err
	}

	if err := u.selectTab(ctx, page, tab); err != nil {
		return nil, err
	}

	return u.extractUserProfileData(page, tab)
}

// selectTab 切到目标子 tab。「笔记」是默认 tab，无需点击。
func (u *UserProfileAction) selectTab(ctx context.Context, page *rod.Page, tab ProfileTab) error {
	if tab == "" || tab == TabNotes {
		return nil
	}

	state, stateErr := page.Eval(`() => { const ref=window.__INITIAL_STATE__?.user?.activeTab; const v=ref?.value??ref?._value??ref; return v?.query||''; }`)
	if stateErr != nil {
		return stateErr
	}
	if state.Value.String() == string(tab) {
		return nil
	}
	label := tabLabel[tab]
	// Server-rendered profile data can arrive before client-rendered tabs.
	// Give tab rendering its own bounded budget instead of treating raw data
	// readiness as proof that interactive controls are already present.
	if err := page.Timeout(15 * time.Second).Wait(rod.Eval(`() => Array.from(document.querySelectorAll('.reds-tab-item.sub-tab-list')).some(el => el.getClientRects().length > 0)`)); err != nil {
		return fmt.Errorf("[profile_tab_unavailable] 主页页签未在限定时间内渲染: %w", err)
	}
	elems, err := page.Elements(`.reds-tab-item.sub-tab-list`)
	if err != nil {
		return fmt.Errorf("未找到主页子 tab: %w", err)
	}

	for _, elem := range elems {
		text, err := elem.Text()
		visible, visibilityErr := elem.Visible()
		if err != nil || visibilityErr != nil || !visible || strings.TrimSpace(text) != label {
			continue
		}
		humanize.Delay(ctx, humanize.BeforeClick)
		if err := humanize.Click(elem); err != nil {
			return fmt.Errorf("切换到 %s 失败: %w", label, err)
		}
		humanize.Delay(ctx, humanize.AfterClick)
		if err := page.Timeout(15 * time.Second).Wait(rod.Eval(`(tab) => {
			const a = window.__INITIAL_STATE__?.user?.activeTab;
			const value = a?.value ?? a?._value ?? a;
			return value && value.query === tab;
		}`, string(tab))); err != nil {
			return fmt.Errorf("[profile_tab_unavailable] 子 tab 未在限定时间内切换: %w", err)
		}
		return nil
	}
	return fmt.Errorf("[profile_tab_unavailable] 未找到可见的目标子 tab，可能未公开")
}

const profilePageStateJS = `() => {
    const visible=x=>!!x?.getClientRects().length;
    if(location.pathname.startsWith('/website-login/')||/^(Security Verification|安全验证)$/i.test(document.title.trim())||visible(document.querySelector('.r-captcha-modal'))) return 'security_verification';
    if(visible(document.querySelector('.access-wrapper'))) return 'profile_access_denied';
    const user=window.__INITIAL_STATE__?.user;
    const ref=user?.userPageData, value=ref?.value??ref?._value??ref;
    const notes=user?.notes,items=notes?.value??notes?._value??notes;
    if(value?.basicInfo && Array.isArray(items)) return 'ready';
    if(visible(document.querySelector('.login-container'))) return 'login_required';
    return 'pending';
}`

func waitProfileData(page *rod.Page) error {
	return waitProfileDataWithin(page, 15*time.Second)
}
func waitProfileDataWithin(page *rod.Page, timeout time.Duration) error {
	pp := page.Timeout(timeout)
	for {
		r, err := pp.Eval(profilePageStateJS)
		if err != nil {
			if page.GetContext().Err() != nil {
				return page.GetContext().Err()
			}
			return fmt.Errorf("[profile_not_ready] 无法读取目标主页状态: %w", err)
		}
		switch state := r.Value.String(); state {
		case "ready":
			return nil
		case "security_verification", "login_required", "profile_access_denied":
			return fmt.Errorf("[%s] 目标主页当前不可读取", state)
		}
		select {
		case <-page.GetContext().Done():
			return page.GetContext().Err()
		case <-pp.GetContext().Done():
			return fmt.Errorf("[profile_not_ready] 目标主页数据未在限定时间内就绪；不能仅据此判定令牌失效")
		case <-time.After(200 * time.Millisecond):
		}
	}
}
