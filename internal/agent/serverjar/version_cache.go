package serverjar

import (
	"context"
	"sync"
	"time"
)

type VersionCache struct {
	mu          sync.RWMutex
	latest      string
	lastChecked time.Time
	lastError   error
	interval    time.Duration
	stopCh      chan struct{}
	stopped     bool
}

func NewVersionCache(interval time.Duration) *VersionCache {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &VersionCache{
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (vc *VersionCache) Start() {
	go vc.loop()
}

func (vc *VersionCache) Stop() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	if !vc.stopped {
		vc.stopped = true
		close(vc.stopCh)
	}
}

func (vc *VersionCache) loop() {
	vc.refresh()
	ticker := time.NewTicker(vc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			vc.refresh()
		case <-vc.stopCh:
			return
		}
	}
}

func (vc *VersionCache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	version, err := ResolveLatestVersion(ctx)
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.lastChecked = time.Now().UTC()
	if err != nil {
		vc.lastError = err
		return
	}
	vc.latest = version
	vc.lastError = nil
}

func (vc *VersionCache) Latest() string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.latest
}

func (vc *VersionCache) CheckedAt() time.Time {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.lastChecked
}

func (vc *VersionCache) LastError() error {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.lastError
}

func (vc *VersionCache) RefreshNow(ctx context.Context) (string, error) {
	version, err := ResolveLatestVersion(ctx)
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.lastChecked = time.Now().UTC()
	if err != nil {
		vc.lastError = err
		return vc.latest, err
	}
	vc.latest = version
	vc.lastError = nil
	return version, nil
}

var defaultVersionCache *VersionCache

func DefaultVersionCache() *VersionCache {
	if defaultVersionCache == nil {
		defaultVersionCache = NewVersionCache(6 * time.Hour)
		defaultVersionCache.Start()
	}
	return defaultVersionCache
}
