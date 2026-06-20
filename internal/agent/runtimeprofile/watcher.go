package runtimeprofile

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Watcher struct {
	mu        sync.Mutex
	registry  *Registry
	dir       string
	interval  time.Duration
	lastCheck map[string]time.Time
	stopCh    chan struct{}
	stopped   bool
}

func NewWatcher(registry *Registry, dir string, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Watcher{
		registry:  registry,
		dir:       dir,
		interval:  interval,
		lastCheck: make(map[string]time.Time),
		stopCh:    make(chan struct{}),
	}
}

func (w *Watcher) Start() {
	go w.loop()
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped {
		w.stopped = true
		close(w.stopCh)
	}
}

func (w *Watcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.scan()
		case <-w.stopCh:
			return
		}
	}
}

func (w *Watcher) scan() {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	currentFiles := make(map[string]time.Time)
	changed := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime()
		currentFiles[entry.Name()] = modTime
		if prev, exists := w.lastCheck[entry.Name()]; !exists || modTime.After(prev) {
			changed = true
		}
	}

	for name := range w.lastCheck {
		if _, exists := currentFiles[name]; !exists {
			changed = true
			break
		}
	}

	if !changed {
		return
	}

	w.lastCheck = currentFiles
	w.reloadAll()
}

func (w *Watcher) reloadAll() {
	var allProfiles []Profile
	for name := range w.lastCheck {
		path := filepath.Join(w.dir, name)
		profiles, err := LoadTrustedFile(path)
		if err != nil {
			log.Printf("runtime-profile watcher: skip %s: %v", name, err)
			continue
		}
		allProfiles = append(allProfiles, profiles...)
	}
	if err := w.registry.Reload(allProfiles); err != nil {
		log.Printf("runtime-profile watcher: reload failed: %v", err)
		return
	}
	log.Printf("runtime-profile watcher: reloaded %d profiles from %d files", len(allProfiles), len(w.lastCheck))
}

func (w *Watcher) ScanNow() {
	w.scan()
}
