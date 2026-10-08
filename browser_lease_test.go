package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserGateCanceledWaitDoesNotConsumeSlot(t *testing.T) {
	g := &browserGate{slots: make(chan struct{}, 1)}
	release, err := g.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := g.acquire(ctx); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request remained queued")
	}
	if len(g.slots) != 1 {
		t.Fatal("canceled request changed active capacity")
	}
	release()
	release()
	if len(g.slots) != 0 {
		t.Fatal("release did not return capacity")
	}
}

func TestQRCodeFailureReturnsBrowserCapacity(t *testing.T) {
	if os.Getenv("BROWSER_TEST_BIN") == "" {
		t.Skip("bundled browser and isolated network required")
	}
	t.Setenv("COOKIES_PATH", filepath.Join(t.TempDir(), "cookies.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	func() {
		defer func() { _ = recover() }()
		_, _ = NewXiaohongshuService().GetLoginQrcode(ctx)
	}()
	if len(browserSlots().slots) != 0 {
		t.Fatal("QR failure retained browser capacity")
	}
}

func TestBrowserGateRejectsAlreadyCanceledRequest(t *testing.T) {
	g := &browserGate{slots: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.slots) != 0 {
		t.Fatal("canceled request acquired capacity")
	}
}

func TestBrowserLeaseReturnsSlotAfterClosePanic(t *testing.T) {
	g := &browserGate{slots: make(chan struct{}, 1)}
	release, _ := g.acquire(context.Background())
	b := &leasedBrowser{release: release}
	func() {
		defer func() { _ = recover() }()
		b.Close()
	}()
	if len(g.slots) != 0 {
		t.Fatal("close failure leaked capacity")
	}
	b.Close()
}
