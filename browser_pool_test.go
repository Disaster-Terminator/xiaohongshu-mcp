package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func setupBrowserPoolTest(t *testing.T) {
	t.Helper()
	if os.Getenv("BROWSER_TEST_BIN") == "" {
		t.Skip("bundled browser required")
	}
	reusableBrowsers.closeIdle()
	t.Setenv("XHS_BROWSER_REUSE", "true")
	t.Setenv("COOKIES_PATH", filepath.Join(t.TempDir(), "cookies.json"))
	t.Cleanup(reusableBrowsers.closeIdle)
}
func TestBrowserPoolReusesOwnerAndDisposesCanceledLease(t *testing.T) {
	setupBrowserPoolTest(t)
	b := newBrowser(context.Background())
	page := b.NewPage()
	engine := page.Browser()
	require.NoError(t, page.Close())
	b.Close()
	b.Close()
	require.Len(t, browserSlots().slots, 0)
	ctx, cancel := context.WithCancel(context.Background())
	again := newBrowser(ctx)
	require.Same(t, b.Browser, again.Browser)
	p := again.NewPage()
	require.NoError(t, p.Close())
	cancel()
	again.Close()
	require.Nil(t, reusableBrowsers.take())
	probe, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_, err := (proto.BrowserGetVersion{}).Call(engine.Context(probe))
	require.Error(t, err)
	require.Len(t, browserSlots().slots, 0)
}
func TestBrowserPoolInvalidatesChangedCookiesAndRejectsLeakedPage(t *testing.T) {
	setupBrowserPoolTest(t)
	b := newBrowser(context.Background())
	p := b.NewPage()
	engine := p.Browser()
	require.NoError(t, p.Close())
	b.Close()
	require.NoError(t, os.WriteFile(os.Getenv("COOKIES_PATH"), []byte("[]"), 0600))
	next := newBrowser(context.Background())
	require.NotSame(t, b.Browser, next.Browser)
	probe, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_, err := (proto.BrowserGetVersion{}).Call(engine.Context(probe))
	require.Error(t, err)
	leaked := next.NewPage()
	require.NoError(t, leaked.Navigate("data:text/html,<html>fixture</html>"))
	next.Close() // An unclosed action page must not cross lease ownership.
	require.Nil(t, reusableBrowsers.take())
	require.Len(t, browserSlots().slots, 0)
}
func TestBrowserPoolDoesNotCreateExtraOwnersAtCapacity(t *testing.T) {
	setupBrowserPoolTest(t)
	a := newBrowser(context.Background())
	b := newBrowser(context.Background())
	ap := a.NewPage()
	bp := b.NewPage()
	require.NoError(t, ap.Close())
	require.NoError(t, bp.Close())
	a.Close()
	b.Close()
	owners := map[any]bool{a.Browser: true, b.Browser: true}
	c := newBrowser(context.Background())
	d := newBrowser(context.Background())
	require.True(t, owners[c.Browser])
	require.True(t, owners[d.Browser])
	require.NotSame(t, c.Browser, d.Browser)
	require.Len(t, browserSlots().slots, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Panics(t, func() { newBrowser(ctx) })
	require.Len(t, browserSlots().slots, 2)
	c.Close()
	d.Close()
	require.Len(t, reusableBrowsers.idle, 2)
	require.Len(t, browserSlots().slots, 0)
}
