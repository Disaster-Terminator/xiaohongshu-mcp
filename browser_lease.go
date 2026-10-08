package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/headless_browser"
)

type browserGate struct{ slots chan struct{} }

func (g *browserGate) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("browser capacity unavailable")
	}
	var once sync.Once
	release := func() { once.Do(func() { <-g.slots }) }
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

var (
	browsers     *browserGate
	browsersOnce sync.Once
)

func browserSlots() *browserGate {
	browsersOnce.Do(func() {
		limit := 2
		if raw := os.Getenv("XHS_BROWSER_CONCURRENCY"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 {
				panic("XHS_BROWSER_CONCURRENCY must be a positive integer")
			}
		}
		browsers = &browserGate{slots: make(chan struct{}, limit)}
	})
	return browsers
}

type leasedBrowser struct {
	*headless_browser.Browser
	release func()
	once    sync.Once
	ctx     context.Context
}

// 建页面也可能卡在 CDP；请求取消时关闭仍由本请求持有的浏览器。
func (b *leasedBrowser) NewPage() *rod.Page {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-b.ctx.Done():
			b.Close()
		case <-done:
		}
	}()
	return b.Browser.NewPage()
}

func (b *leasedBrowser) Close() {
	b.once.Do(func() {
		defer b.release()
		b.Browser.Close()
	})
}
