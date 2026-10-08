package headless_browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

func TestConstructionFailureDoesNotRetainBrowserProcess(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for constructor failure regression")
	}
	dir := t.TempDir()
	func() {
		defer func() {
			failure := recover()
			if failure == nil {
				t.Error("expected invalid cookie construction to fail")
			} else if !strings.Contains(strings.ToLower(fmt.Sprint(failure)), "cookie") {
				t.Fatalf("failed before cookie/CDP fault injection: %v", failure)
			}
		}()
		b := New(WithChromeBinPath(bin), WithHeadless(true),
			WithExtraFlags(map[string]string{"user-data-dir": dir}),
			WithCookies(`[{"name":"invalid","value":"x"}]`))
		defer b.Close()
	}()
	if _, err := os.Stat(filepath.Join(dir, "SingletonLock")); !os.IsNotExist(err) {
		t.Fatal("construction failure retained browser profile/process")
	}
}

func TestBrowserEventsSurviveLaunchContextCancellation(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for lifetime regression")
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := New(WithChromeBinPath(bin), WithHeadless(true), WithLaunchContext(ctx))
	defer b.Close()
	cancel()
	page := b.NewPage().Timeout(5 * time.Second)
	defer page.Close()
	wait := page.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
	page.MustNavigate("data:text/html,<html><body>ready</body></html>")
	wait()
	if page.MustElement("body").MustText() != "ready" {
		t.Fatal("browser events lost after ownership handoff")
	}
}

func TestCloseTerminatesBrowserAfterCDPFailure(t *testing.T) {
	bin := os.Getenv("BROWSER_TEST_BIN")
	if bin == "" {
		t.Skip("BROWSER_TEST_BIN required for close failure regression")
	}
	b := New(WithChromeBinPath(bin), WithHeadless(true))
	dir := b.launcher.Get(flags.UserDataDir)
	b.launcher.Kill()
	b.Close()
	b.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("close failure skipped profile cleanup")
	}
}
