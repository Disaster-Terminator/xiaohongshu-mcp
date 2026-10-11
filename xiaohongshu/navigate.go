package xiaohongshu

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

type NavigateAction struct {
	page *rod.Page
}

func NewNavigate(page *rod.Page) *NavigateAction {
	return &NavigateAction{page: page}
}

func (n *NavigateAction) ToExplorePage(ctx context.Context) error {
	page := n.page.Context(ctx).Timeout(60 * time.Second) // 加超时保护，避免 MustNavigate/MustWaitStable 无限挂

	navigateDocument(page, "https://www.xiaohongshu.com/explore")
	page.MustElement(`div#app`)

	return nil
}

func (n *NavigateAction) ToProfilePage(ctx context.Context) error {
	return n.toProfileFromHome(ctx, "https://www.xiaohongshu.com/explore")
}

func (n *NavigateAction) toProfileFromHome(ctx context.Context, homeURL string) error {
	page := n.page.Context(ctx).Timeout(60 * time.Second)
	if err := navigateDocumentChecked(page.Timeout(15*time.Second), homeURL); err != nil {
		return err
	}
	if err := requireAuthenticated(page); err != nil {
		return err
	}
	target, err := page.Eval(`() => {
        const ref=window.__INITIAL_STATE__?.user?.userInfo;
        const u=ref?.value??ref?._value??ref;
        const link=document.querySelector('.main-container .user a.link-wrapper[href*="/user/profile/"]');
        const href=link?.getAttribute('href');
        const id=u?.userId||u?.user_id;
        const url=new URL(href||('/user/profile/'+encodeURIComponent(id||'')),location.href);
        return id && url.origin===location.origin && url.pathname==='/user/profile/'+id ? url.href : '';
    }`)
	if err != nil {
		return err
	}
	if target.Value.String() == "" {
		return fmt.Errorf("[profile_navigation] 无法确认自己的主页地址")
	}
	if err := navigateDocumentChecked(page.Timeout(15*time.Second), target.Value.String()); err != nil {
		return err
	}
	if err := requireAuthenticated(page); err != nil {
		return err
	}
	return waitProfileData(page)
}
