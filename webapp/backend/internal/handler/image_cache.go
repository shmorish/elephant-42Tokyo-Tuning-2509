package handler

import (
	"sync"
	"time"
)

// ImageCacheEntry 画像キャッシュのエントリ
type ImageCacheEntry struct {
	Data        []byte
	ContentType string
	Timestamp   time.Time
	Size        int64
}

// ImageCache 画像のメモリキャッシュ
type ImageCache struct {
	cache    map[string]ImageCacheEntry
	mutex    sync.RWMutex
	maxSize  int64                    // 最大キャッシュサイズ（バイト）
	maxAge   time.Duration           // キャッシュの最大有効期限
	cleanup  time.Duration           // クリーンアップ間隔
	stopChan chan struct{}
}

// NewImageCache 新しい画像キャッシュを作成
func NewImageCache(maxSize int64, maxAge time.Duration) *ImageCache {
	cache := &ImageCache{
		cache:    make(map[string]ImageCacheEntry),
		maxSize:  maxSize,
		maxAge:   maxAge,
		cleanup:  5 * time.Minute, // 5分ごとにクリーンアップ
		stopChan: make(chan struct{}),
	}
	
	// バックグラウンドでクリーンアップを開始
	go cache.startCleanup()
	
	return cache
}

// Get キャッシュから画像を取得
func (c *ImageCache) Get(key string) ([]byte, string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	entry, exists := c.cache[key]
	if !exists {
		return nil, "", false
	}
	
	// 有効期限チェック
	if time.Since(entry.Timestamp) > c.maxAge {
		return nil, "", false
	}
	
	return entry.Data, entry.ContentType, true
}

// Set キャッシュに画像を保存
func (c *ImageCache) Set(key string, data []byte, contentType string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	// サイズ制限チェック
	if int64(len(data)) > c.maxSize {
		return // サイズが大きすぎる場合はキャッシュしない
	}
	
	// キャッシュサイズ制限をチェック
	if c.shouldEvict(int64(len(data))) {
		c.evictOldest()
	}
	
	c.cache[key] = ImageCacheEntry{
		Data:        data,
		ContentType: contentType,
		Timestamp:   time.Now(),
		Size:        int64(len(data)),
	}
}

// shouldEvict キャッシュサイズが制限を超えているかチェック
func (c *ImageCache) shouldEvict(newSize int64) bool {
	totalSize := int64(0)
	for _, entry := range c.cache {
		totalSize += entry.Size
	}
	return totalSize+newSize > c.maxSize
}

// evictOldest 最も古いエントリを削除
func (c *ImageCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, entry := range c.cache {
		if oldestKey == "" || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
		}
	}
	
	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// startCleanup バックグラウンドでクリーンアップを実行
func (c *ImageCache) startCleanup() {
	ticker := time.NewTicker(c.cleanup)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopChan:
			return
		}
	}
}

// cleanupExpired 期限切れのエントリを削除
func (c *ImageCache) cleanupExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	now := time.Now()
	for key, entry := range c.cache {
		if now.Sub(entry.Timestamp) > c.maxAge {
			delete(c.cache, key)
		}
	}
}

// Stop キャッシュのクリーンアップを停止
func (c *ImageCache) Stop() {
	close(c.stopChan)
}

// GetStats キャッシュの統計情報を取得
func (c *ImageCache) GetStats() (int, int64) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	count := len(c.cache)
	totalSize := int64(0)
	for _, entry := range c.cache {
		totalSize += entry.Size
	}
	
	return count, totalSize
}
