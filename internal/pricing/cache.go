package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheManager handles hybrid in-memory and disk caching for cost data.
type CacheManager struct {
	memoryCache map[string]CacheEntry
	cacheDir    string
	mu          sync.RWMutex
	ttl         time.Duration
}

// NewCacheManager creates a new CacheManager.
// cacheDir is the directory to store persistent cache files.
// ttl is the time-to-live for cache entries.
func NewCacheManager(cacheDir string, ttl time.Duration) (*CacheManager, error) {
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting user home dir: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".pulumicost", "cache", "aws-ce")
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	cm := &CacheManager{
		memoryCache: make(map[string]CacheEntry),
		cacheDir:    cacheDir,
		ttl:         ttl,
	}

	// Load from disk on startup
	if err := cm.loadFromDisk(); err != nil {
		// Log error but continue with empty cache
		fmt.Fprintf(os.Stderr, "Failed to load cache from disk: %v\n", err)
	}

	return cm, nil
}

// Get retrieves a cache entry by key.
func (cm *CacheManager) Get(key string) ([]CostEntry, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	entry, ok := cm.memoryCache[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Results, true
}

// Set stores a cache entry.
func (cm *CacheManager) Set(key string, results []CostEntry) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	entry := CacheEntry{
		QueryKey:  key,
		Results:   results,
		CreatedAt: now,
		ExpiresAt: now.Add(cm.ttl),
	}

	cm.memoryCache[key] = entry
	return cm.saveToDisk(key, entry)
}

// saveToDisk writes a single cache entry to disk.
func (cm *CacheManager) saveToDisk(key string, entry CacheEntry) error {
	filename := filepath.Join(cm.cacheDir, fmt.Sprintf("%s.json", key))
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling cache entry: %w", err)
	}

	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	return nil
}

// loadFromDisk loads all valid cache entries from the cache directory.
func (cm *CacheManager) loadFromDisk() error {
	entries, err := os.ReadDir(cm.cacheDir)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		path := filepath.Join(cm.cacheDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}

		// Check freshness using file modification time if needed,
		// but we have ExpiresAt in the struct.
		// FR-013 mentions "uses filesystem timestamps to determine cache freshness".
		// We can check file mod time here as a double check or primary check.
		info, err := e.Info()
		if err == nil {
			if time.Since(info.ModTime()) > cm.ttl {
				// Expired based on file time, clean up
				_ = os.Remove(path)
				continue
			}
		}

		if time.Now().After(entry.ExpiresAt) {
			_ = os.Remove(path)
			continue
		}

		cm.memoryCache[entry.QueryKey] = entry
	}

	return nil
}
