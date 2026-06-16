package filelock

import (
	"path/filepath"
	"sync"
	"time"
)

type lockEntry struct {
	mu       sync.RWMutex
	lastUsed int64
}

var (
	globalMu  sync.Mutex
	fileLocks = map[string]*lockEntry{}
)

func RLockFile(path string) {
	entry := getEntry(path)
	entry.mu.RLock()
	touch(entry)
}

func RUnlockFile(path string) {
	entry := getEntry(path)
	entry.mu.RUnlock()
	touch(entry)
}

func LockFile(path string) {
	entry := getEntry(path)
	entry.mu.Lock()
	touch(entry)
}

func UnlockFile(path string) {
	entry := getEntry(path)
	entry.mu.Unlock()
	touch(entry)
}

func StartCleanup(interval time.Duration, maxIdle time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			cutoff := time.Now().Add(-maxIdle).Unix()
			globalMu.Lock()
			for path, entry := range fileLocks {
				if entry.lastUsed < cutoff {
					delete(fileLocks, path)
				}
			}
			globalMu.Unlock()
		}
	}()
}

func getEntry(path string) *lockEntry {
	abs := filepath.Clean(path)
	globalMu.Lock()
	defer globalMu.Unlock()
	entry, ok := fileLocks[abs]
	if !ok {
		entry = &lockEntry{lastUsed: time.Now().Unix()}
		fileLocks[abs] = entry
	}
	return entry
}

func touch(entry *lockEntry) {
	globalMu.Lock()
	entry.lastUsed = time.Now().Unix()
	globalMu.Unlock()
}
