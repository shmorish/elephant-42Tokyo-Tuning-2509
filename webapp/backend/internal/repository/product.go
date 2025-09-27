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
	db    DBTX
	sf    singleflight.Group
	cache map[string]cacheEntry
	mutex sync.RWMutex
	ttl   time.Duration
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{
		db:    db,
		cache: make(map[string]cacheEntry),
		ttl:   5 * time.Minute, // 5分キャッシュ
	}
}

// 商品一覧をDBレベルでページングして取得（キャッシュ＋シングルフライト対応）
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	// Create unique key for cache and singleflight
	key := fmt.Sprintf("products:%s:%s:%s:%d:%d", req.Search, req.SortField, req.SortOrder, req.PageSize, req.Offset)

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
	products []model.Product
	total    int
}

func (r *ProductRepository) listProductsInternal(ctx context.Context, userID int, req model.ListRequest) (productResult, error) {
	var products []model.Product
	var total int

	// Count query for total records
	countQuery := "SELECT COUNT(*) FROM products"
	countArgs := []interface{}{}

	if req.Search != "" {
		countQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		countArgs = append(countArgs, searchPattern, searchPattern)
	}

	err := r.db.GetContext(ctx, &total, countQuery, countArgs...)
	if err != nil {
		return productResult{}, err
	}

	// Data query with pagination
	dataQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}

	if req.Search != "" {
		dataQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	dataQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	dataQuery += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	err = r.db.SelectContext(ctx, &products, dataQuery, args...)
	if err != nil {
		return productResult{}, err
	}

	return productResult{products: products, total: total}, nil
}
