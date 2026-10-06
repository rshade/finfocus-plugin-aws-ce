package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
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
		cacheDir = filepath.Join(homeDir, ".finfocus", "cache", "aws-ce")
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

const (
	closedHistoryTTL   = 24 * time.Hour
	recentCostTTL      = 15 * time.Minute
	recentCostHorizon  = 48 * time.Hour
	cacheGranularity   = "DAILY"
	cacheMetric        = "UnblendedCost"
	cacheGroupService  = "SERVICE"
	cacheGroupResource = "RESOURCE_ID"
)

// Get retrieves a cache entry by key.
func (cm *CacheManager) Get(key string) ([]CostEntry, bool) {
	entry, ok := cm.getEntry(key)
	if !ok {
		return nil, false
	}
	return entry.Results, true
}

func (cm *CacheManager) getEntry(key string) (CacheEntry, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	entry, ok := cm.memoryCache[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return CacheEntry{}, false
	}
	return entry, true
}

// Set stores a cache entry for cm.ttl.
func (cm *CacheManager) Set(key string, results []CostEntry) error {
	_, err := cm.SetWithTTL(key, results, cm.ttl)
	return err
}

// SetWithTTL stores a cache entry that expires after ttl.
// ttl applies only to this entry. It does not change the manager default.
func (cm *CacheManager) SetWithTTL(key string, results []CostEntry, ttl time.Duration) (time.Time, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now().UTC()
	expires := now.Add(ttl)
	entry := CacheEntry{
		QueryKey:  key,
		Results:   results,
		CreatedAt: now,
		ExpiresAt: expires,
	}

	cm.memoryCache[key] = entry
	if err := cm.saveToDisk(key, entry); err != nil {
		return expires, err
	}
	return expires, nil
}

// cacheFileName is the SHA-256 hex of the logical key plus ".json".
// The key can contain '/' and '..', so it is not used as a path segment.
func cacheFileName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]) + ".json"
}

// saveToDisk writes a single cache entry to disk.
func (cm *CacheManager) saveToDisk(key string, entry CacheEntry) error {
	filename := filepath.Join(cm.cacheDir, cacheFileName(key))
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

// actualCostCacheKey identifies one GetActualCost answer.
// Tags are not in the key: GetActualCost ignores them (CE-6.7).
func actualCostCacheKey(plan costQueryPlan, req *pbc.GetActualCostRequest) string {
	group := cacheGroupService
	if plan.resourceLevel {
		group = cacheGroupResource
	}
	return fmt.Sprintf("cost:%s:%d:%d:%s:%s:%s",
		plan.cacheID,
		req.GetStart().GetSeconds(),
		req.GetEnd().GetSeconds(),
		cacheGranularity,
		group,
		cacheMetric,
	)
}

// actualCostTTL is 15 minutes when a row is estimated or the request still
// ends inside the last 48 hours. Closed history with no estimate stays 24 hours.
func actualCostTTL(end time.Time, rows []CostEntry, now time.Time) time.Duration {
	for _, row := range rows {
		if row.Estimated {
			return recentCostTTL
		}
	}
	if end.After(now.UTC().Add(-recentCostHorizon)) {
		return recentCostTTL
	}
	return closedHistoryTTL
}
