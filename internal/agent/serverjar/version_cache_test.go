package serverjar

import (
	"testing"
	"time"
)

func TestVersionCacheStartAndLatest(t *testing.T) {
	cache := NewVersionCache(1 * time.Hour)
	defer cache.Stop()

	version, err := cache.RefreshNow(t.Context())
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty version")
	}

	cache.Start()
	time.Sleep(100 * time.Millisecond)

	latest := cache.Latest()
	if latest != version {
		t.Fatalf("latest = %q after start, want %q", latest, version)
	}
	t.Logf("cached latest: %s (checked at %s)", latest, cache.CheckedAt().Format(time.RFC3339))
}

func TestVersionCacheRefreshNow(t *testing.T) {
	cache := NewVersionCache(1 * time.Hour)
	defer cache.Stop()

	version, err := cache.RefreshNow(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("expected non-empty version")
	}
	if cache.Latest() != version {
		t.Fatalf("latest = %q, want %q", cache.Latest(), version)
	}
}

func TestVersionCacheStopPreventsPolling(t *testing.T) {
	cache := NewVersionCache(50 * time.Millisecond)
	cache.Start()

	// Wait for at least one successful poll so first is non-empty.
	var first string
	for range 30 {
		time.Sleep(50 * time.Millisecond)
		first = cache.Latest()
		if first != "" {
			break
		}
	}
	if first == "" {
		t.Fatal("cache did not populate after start")
	}

	cache.Stop()
	time.Sleep(200 * time.Millisecond)

	if cache.Latest() != first {
		t.Fatalf("version changed after stop: %q -> %q", first, cache.Latest())
	}
}
