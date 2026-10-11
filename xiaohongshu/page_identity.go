package xiaohongshu

import (
	"fmt"
	"github.com/go-rod/rod"
	"time"
)

// Guest users also have user IDs. Neither an ID nor a sidebar template alone
// proves authentication. Keep unknown/hydrating state distinct from signed out.
const authenticationStateJS = `() => {
 const visible=x=>!!x?.getClientRects().length;
 if(location.pathname.startsWith('/website-login/') || /^(Security Verification|安全验证)$/i.test(document.title.trim()) || visible(document.querySelector('.r-captcha-modal'))) return 'security_verification';
 const store=window.__INITIAL_STATE__?.user;
 const ref=store?.userInfo, u=ref?.value??ref?._value??ref;
 if(u?.guest===true) return 'login_required';
 const logged=store?.loggedIn?.value??store?.loggedIn?._value??store?.loggedIn;
 if((u?.guest===false||logged===true) && !!(u?.userId||u?.user_id)) return 'authenticated';
 if(visible(document.querySelector('.login-container'))) return 'login_required';
 return 'pending';
}`

func waitAuthenticated(page *rod.Page, timeout time.Duration) (bool, error) {
	pp := page.Timeout(timeout)
	for {
		r, err := pp.Eval(authenticationStateJS)
		if err != nil {
			if page.GetContext().Err() != nil {
				return false, page.GetContext().Err()
			}
			return false, fmt.Errorf("[auth_state_not_ready] 无法确认登录身份: %w", err)
		}
		switch r.Value.String() {
		case "authenticated":
			return true, nil
		case "login_required":
			return false, nil
		case "security_verification":
			return false, fmt.Errorf("[security_verification] 页面要求安全验证")
		}
		select {
		case <-page.GetContext().Done():
			return false, page.GetContext().Err()
		case <-pp.GetContext().Done():
			return false, fmt.Errorf("[auth_state_not_ready] 登录身份未在限定时间内就绪")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func requireAuthenticated(page *rod.Page) error {
	authenticated, err := waitAuthenticated(page, 12*time.Second)
	if err != nil {
		return err
	}
	if !authenticated {
		return fmt.Errorf("[login_required] 当前会话未登录")
	}
	return nil
}
