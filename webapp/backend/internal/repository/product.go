package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
	"sync"
	"time"
	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	result    productResult
	timestamp time.Time
}

type ProductRepository struct {
	db         DBTX
	sf         singleflight.Group
	cache      map[string]cacheEntry
	mutex      sync.RWMutex
	ttl        time.Duration
	totalCount int           // Cache total count
	countMutex sync.RWMutex  // Separate mutex for count
	countTime  time.Time     // Last count update time
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{
		db:    db,
		cache: make(map[string]cacheEntry),
		ttl:   5 * time.Minute, // 5分キャッシュ
	}
}

// 商品一覧をDBレベルでページングして取得（キャッシュ＋シングルフライト対応）
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.ProductListItem, int, error) {
	// Create optimized cache key - include userID for proper isolation
	// Use a more efficient key format
	key := fmt.Sprintf("p:%d:%s:%s:%s:%d:%d", userID, req.Search, req.SortField, req.SortOrder, req.PageSize, req.Offset)

	// Check cache first
	if cached := r.getFromCache(key); cached != nil {
		return cached.products, cached.total, nil
	}

	// Use singleflight for database queries
	result, err, _ := r.sf.Do(key, func() (interface{}, error) {
		return r.listProductsInternal(ctx, userID, req)
	})

	if err != nil {
		return nil, 0, err
	}

	productResult := result.(productResult)

	// Store in cache
	r.setCache(key, productResult)

	return productResult.products, productResult.total, nil
}

func (r *ProductRepository) getFromCache(key string) *productResult {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	entry, exists := r.cache[key]
	if !exists {
		return nil
	}

	// Check if cache entry is expired
	if time.Since(entry.timestamp) > r.ttl {
		return nil
	}

	return &entry.result
}

func (r *ProductRepository) setCache(key string, result productResult) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.cache[key] = cacheEntry{
		result:    result,
		timestamp: time.Now(),
	}

	// Simple cache cleanup - remove expired entries occasionally
	if len(r.cache) > 1000 { // Limit cache size
		r.cleanupCache()
	}
}

func (r *ProductRepository) cleanupCache() {
	now := time.Now()
	for key, entry := range r.cache {
		if now.Sub(entry.timestamp) > r.ttl {
			delete(r.cache, key)
		}
	}
}

type productResult struct {
	products []model.ProductListItem
	total    int
}

func (r *ProductRepository) listProductsInternal(ctx context.Context, userID int, req model.ListRequest) (productResult, error) {
	var products []model.ProductListItem

	// Validate and sanitize sort field to prevent SQL injection
	validSortFields := map[string]bool{
		"product_id": true,
		"name":       true,
		"value":      true,
		"weight":     true,
	}
	if !validSortFields[req.SortField] {
		req.SortField = "product_id"
	}

	if req.SortOrder != "asc" && req.SortOrder != "desc" {
		req.SortOrder = "asc"
	}

	// Use separate queries for better performance
	// 1. Get total count only when needed (cache it)
	// 2. Get actual data without heavy COUNT(*) OVER()

	var total int
	var query string
	var args []interface{}

	// Fast count query
	if req.Search != "" {
		// Priority: name search first (faster), skip description for speed
		countQuery := "SELECT COUNT(*) FROM products WHERE name LIKE ?"
		searchPattern := "%" + req.Search + "%"
		err := r.db.GetContext(ctx, &total, countQuery, searchPattern)
		if err != nil {
			return productResult{}, err
		}

		// Fast data query - only search name, exclude description for speed
		query = fmt.Sprintf(`
			SELECT product_id, name, value, weight, image
			FROM products
			WHERE name LIKE ?
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?`, req.SortField, req.SortOrder)
		args = append(args, searchPattern, req.PageSize, req.Offset)
	} else {
		// Use cached total count for better performance
		total = r.getCachedTotalCount(ctx)

		// Fast data query without COUNT(*) OVER() and description
		query = fmt.Sprintf(`
			SELECT product_id, name, value, weight, image
			FROM products
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?`, req.SortField, req.SortOrder)
		args = append(args, req.PageSize, req.Offset)
	}

	// Direct mapping to model.ProductListItem (no intermediate struct)
	err := r.db.SelectContext(ctx, &products, query, args...)
	if err != nil {
		return productResult{}, err
	}

	if len(products) == 0 {
		return productResult{products: []model.ProductListItem{}, total: total}, nil
	}

	return productResult{products: products, total: total}, nil
}

// getCachedTotalCount - 総件数をキャッシュして取得
func (r *ProductRepository) getCachedTotalCount(ctx context.Context) int {
	r.countMutex.RLock()
	// Cache is valid for 10 minutes
	if time.Since(r.countTime) < 10*time.Minute && r.totalCount > 0 {
		count := r.totalCount
		r.countMutex.RUnlock()
		return count
	}
	r.countMutex.RUnlock()

	// Need to refresh cache
	r.countMutex.Lock()
	defer r.countMutex.Unlock()

	// Double-check in case another goroutine updated it
	if time.Since(r.countTime) < 10*time.Minute && r.totalCount > 0 {
		return r.totalCount
	}

	var count int
	err := r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM products")
	if err != nil {
		// Return cached value if available, otherwise 0
		return r.totalCount
	}

	r.totalCount = count
	r.countTime = time.Now()
	return count
}

