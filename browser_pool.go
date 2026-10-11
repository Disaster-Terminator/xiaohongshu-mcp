package main

import (
	"context"
	"crypto/sha256"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/headless_browser"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
)

// Idle processes share the same capacity as active leases. Checkout always
// consumes an idle entry before constructing a new process; return stores the
// entry before releasing admission. Thus active + idle never exceeds the gate.
type pooledBrowser struct {
	browser *headless_browser.Browser
	engine  atomic.Pointer[rod.Browser]
	session [32]byte
}
type browserPool struct {
	mu   sync.Mutex
	idle []*pooledBrowser
}

var reusableBrowsers browserPool

func browserSession() ([32]byte, error) {
	data, err := os.ReadFile(cookies.GetCookiesFilePath())
	if os.IsNotExist(err) {
		return sha256.Sum256(nil), nil
	}
	return sha256.Sum256(data), err
}
func (p *browserPool) take() *pooledBrowser {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.idle) == 0 {
		return nil
	}
	i := len(p.idle) - 1
	b := p.idle[i]
	p.idle[i] = nil
	p.idle = p.idle[:i]
	return b
}
func (p *browserPool) put(b *pooledBrowser) { p.mu.Lock(); p.idle = append(p.idle, b); p.mu.Unlock() }
func (p *browserPool) closeIdle() {
	for {
		b := p.take()
		if b == nil {
			return
		}
		b.browser.Close()
	}
}
func (b *pooledBrowser) alive(ctx context.Context) bool {
	engine := b.engine.Load()
	if engine == nil {
		return false
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := (proto.BrowserGetVersion{}).Call(engine.Context(probe))
	return err == nil
}
func (b *pooledBrowser) canReturn(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	session, err := browserSession()
	if err != nil || session != b.session {
		return false
	}
	engine := b.engine.Load()
	if engine == nil {
		return false
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	pages, err := engine.Context(probe).Pages()
	if err != nil {
		return false
	}
	if len(pages) > 1 {
		return false
	} // Only the launcher's initial blank tab may remain.
	for _, page := range pages {
		info, e := page.Info()
		if e != nil || info.URL != "about:blank" {
			return false
		}
	}
	return ctx.Err() == nil
}
