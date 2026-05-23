package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"one-mcp/backend/model"
	"sync"
	"time"

	"github.com/burugo/thing"
	"github.com/mark3labs/mcp-go/mcp"
)

type ToolsCacheEntry struct {
	Tools     []mcp.Tool `json:"tools"`
	FetchedAt time.Time  `json:"fetched_at"`
}

type toolsLocalCacheItem struct {
	value     string
	expiresAt time.Time
}

// ToolsCacheManager caches tool lists separately from health status.
type ToolsCacheManager struct {
	cacheClient thing.CacheClient
	expireTime  time.Duration
	mutex       sync.RWMutex
	local       map[string]toolsLocalCacheItem
}

func NewToolsCacheManager(expireTime time.Duration) *ToolsCacheManager {
	if expireTime <= 0 {
		expireTime = 10 * time.Minute
	}

	return &ToolsCacheManager{
		cacheClient: thing.Cache(),
		expireTime:  expireTime,
		local:       make(map[string]toolsLocalCacheItem),
	}
}

func (tcm *ToolsCacheManager) generateCacheKey(serviceID int64) string {
	return fmt.Sprintf("tools:service:%d", serviceID)
}

func (tcm *ToolsCacheManager) SetServiceTools(serviceID int64, entry *ToolsCacheEntry) {
	if entry == nil {
		return
	}

	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()

	ctx := context.Background()
	cacheKey := tcm.generateCacheKey(serviceID)

	entryCopy := *entry
	entryJSON, err := json.Marshal(&entryCopy)
	if err != nil {
		log.Printf("Error marshaling tools cache for service %d: %v", serviceID, err)
		return
	}

	if tcm.cacheClient == nil {
		tcm.local[cacheKey] = toolsLocalCacheItem{
			value:     string(entryJSON),
			expiresAt: time.Now().Add(tcm.expireTime),
		}
		return
	}

	if err := tcm.cacheClient.Set(ctx, cacheKey, string(entryJSON), tcm.expireTime); err != nil {
		log.Printf("Error setting tools cache for service %d: %v", serviceID, err)
		return
	}
}

func (tcm *ToolsCacheManager) GetServiceTools(serviceID int64) (*ToolsCacheEntry, bool) {
	tcm.mutex.RLock()
	defer tcm.mutex.RUnlock()

	ctx := context.Background()
	cacheKey := tcm.generateCacheKey(serviceID)

	var entryJSON string
	if tcm.cacheClient == nil {
		item, ok := tcm.local[cacheKey]
		if !ok {
			return nil, false
		}
		if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
			delete(tcm.local, cacheKey)
			return nil, false
		}
		entryJSON = item.value
	} else {
		v, err := tcm.cacheClient.Get(ctx, cacheKey)
		if err != nil {
			return nil, false
		}
		entryJSON = v
	}

	var entry ToolsCacheEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		log.Printf("Error unmarshaling tools cache for service %d: %v", serviceID, err)
		go tcm.DeleteServiceTools(serviceID)
		return nil, false
	}

	return &entry, true
}

func (tcm *ToolsCacheManager) DeleteServiceTools(serviceID int64) {
	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()

	ctx := context.Background()
	cacheKey := tcm.generateCacheKey(serviceID)

	if tcm.cacheClient == nil {
		delete(tcm.local, cacheKey)
		return
	}

	if err := tcm.cacheClient.Delete(ctx, cacheKey); err != nil {
		log.Printf("Error deleting tools cache for service %d: %v", serviceID, err)
	}
}

var globalToolsCacheManager *ToolsCacheManager
var toolsCacheOnce sync.Once

// OnToolsCached is called after a service's tools are cached or refreshed.
// Handler code can set this to clear stale handler caches that depend on tool lists.
var OnToolsCached func(serviceID int64)

func GetToolsCacheManager() *ToolsCacheManager {
	toolsCacheOnce.Do(func() {
		globalToolsCacheManager = NewToolsCacheManager(10 * time.Minute)
	})
	return globalToolsCacheManager
}

// RefreshServiceTools fetches tools from a service and writes them to the cache.
// Runs asynchronously in a goroutine. Errors are logged but not returned to the caller.
func RefreshServiceTools(ctx context.Context, svc *model.MCPService) {
	if svc == nil {
		return
	}
	go func() {
		sharedInst, err := GetOrCreateSharedMcpInstanceWithKey(ctx, svc,
			SharedServiceCacheKey(svc.ID), SharedServiceInstanceName(svc.ID), svc.DefaultEnvsJSON)
		if err != nil {
			log.Printf("[ToolsCache] Failed to get shared instance for %s: %v", svc.Name, err)
			return
		}
		toolsReq := mcp.ListToolsRequest{}
		result, err := sharedInst.Client.ListTools(ctx, toolsReq)
		if err != nil {
			log.Printf("[ToolsCache] Failed to list tools for %s: %v", svc.Name, err)
			return
		}
		if result == nil {
			return
		}
		GetToolsCacheManager().SetServiceTools(svc.ID, &ToolsCacheEntry{
			Tools:     result.Tools,
			FetchedAt: time.Now(),
		})
		if OnToolsCached != nil {
			OnToolsCached(svc.ID)
		}
		log.Printf("[ToolsCache] Refreshed %d tools for %s (ID: %d)", len(result.Tools), svc.Name, svc.ID)
	}()
}

// StartPeriodicRefresh starts a background goroutine that periodically refreshes all cached tool entries.
// The ticker runs on the configured expireTime interval. Pass context cancellation to stop.
func StartPeriodicRefresh(ctx context.Context) {
	tcm := GetToolsCacheManager()
	if tcm.expireTime <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(tcm.expireTime)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tcm.mutex.RLock()
				keys := make([]string, 0, len(tcm.local))
				for k := range tcm.local {
					keys = append(keys, k)
				}
				tcm.mutex.RUnlock()
				for _, key := range keys {
					// Extract service ID from cache key "tools:service:{id}"
					var svcID int64
					if _, err := fmt.Sscanf(key, "tools:service:%d", &svcID); err == nil && svcID > 0 {
						svc, err := model.GetServiceByID(svcID)
						if err == nil && svc.Enabled {
							RefreshServiceTools(ctx, svc)
						}
					}
				}
			}
		}
	}()
}
