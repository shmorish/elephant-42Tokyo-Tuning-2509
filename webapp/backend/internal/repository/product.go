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
	products []model.Product
	total    int
}

func (r *ProductRepository) listProductsInternal(ctx context.Context, userID int, req model.ListRequest) (productResult, error) {
	var products []model.Product

	// Build optimized query with proper indexing support
	var query string
	var args []interface{}

	// Validate and sanitize sort field to prevent SQL injection
	validSortFields := map[string]bool{
		"product_id": true,
		"name":       true,
		"value":      true,
		"weight":     true,
	}
	if !validSortFields[req.SortField] {
		req.SortField = "product_id" // fallback to safe default
	}

	// Validate sort order
	if req.SortOrder != "asc" && req.SortOrder != "desc" {
		req.SortOrder = "asc"
	}

	if req.Search != "" {
		// Use optimized LIKE search for compatibility
		// TODO: Consider FULLTEXT search after proper index verification
		query = fmt.Sprintf(`
			SELECT
				product_id, name, value, weight, image, description,
				COUNT(*) OVER() as total_count
			FROM products
			WHERE (name LIKE ? OR description LIKE ?)
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?`, req.SortField, req.SortOrder)
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern, req.PageSize, req.Offset)
	} else {
		// No search: use covering index for optimal performance
		query = fmt.Sprintf(`
			SELECT
				product_id, name, value, weight, image, description,
				COUNT(*) OVER() as total_count
			FROM products
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?`, req.SortField, req.SortOrder)
		args = append(args, req.PageSize, req.Offset)
	}

	type productRowWithCount struct {
		ProductID   int    `db:"product_id"`
		Name        string `db:"name"`
		Value       int    `db:"value"`
		Weight      int    `db:"weight"`
		Image       string `db:"image"`
		Description string `db:"description"`
		TotalCount  int    `db:"total_count"`
	}

	var productsRaw []productRowWithCount
	err := r.db.SelectContext(ctx, &productsRaw, query, args...)
	if err != nil {
		return productResult{}, err
	}

	if len(productsRaw) == 0 {
		return productResult{products: []model.Product{}, total: 0}, nil
	}

	// 最初の行からtotal_countを取得
	total := productsRaw[0].TotalCount

	products = make([]model.Product, len(productsRaw))
	for i, p := range productsRaw {
		products[i] = model.Product{
			ProductID:   p.ProductID,
			Name:        p.Name,
			Value:       p.Value,
			Weight:      p.Weight,
			Image:       p.Image,
			Description: p.Description,
		}
	}

	return productResult{products: products, total: total}, nil
}

