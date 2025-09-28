package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ProductHandler struct {
	ProductSvc *service.ProductService
	ImageCache *ImageCache
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	// 画像キャッシュの設定
	// 最大サイズ: 100MB, 有効期限: 1時間
	imageCache := NewImageCache(100*1024*1024, time.Hour)
	
	return &ProductHandler{
		ProductSvc: svc,
		ImageCache: imageCache,
	}
}

// 商品一覧を取得
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	var req model.ListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.SortField == "" {
		req.SortField = "product_id"
	}
	if req.SortOrder == "" {
		req.SortOrder = "asc"
	}
	req.Offset = (req.Page - 1) * req.PageSize

	products, total, err := h.ProductSvc.FetchProducts(r.Context(), userID, req)
	if err != nil {
		log.Printf("Failed to fetch products for user %d: %v", userID, err)
		http.Error(w, "Failed to fetch products", http.StatusInternalServerError)
		return
	}

	resp := struct {
		Data  []model.Product `json:"data"`
		Total int             `json:"total"`
	}{
		Data:  products,
		Total: total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// 注文を作成
func (h *ProductHandler) CreateOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	var req model.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	insertedOrderIDs, err := h.ProductSvc.CreateOrders(r.Context(), userID, req.Items)
	if err != nil {
		log.Printf("Failed to create orders: %v", err)
		http.Error(w, "Failed to process order request", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"message":   "Orders created successfully",
		"order_ids": insertedOrderIDs,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (h *ProductHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	imagePath := r.URL.Query().Get("path")
	if imagePath == "" {
		http.Error(w, "画像パスが指定されていません", http.StatusBadRequest)
		return
	}

	imagePath = filepath.Clean(imagePath)
	if filepath.IsAbs(imagePath) || strings.Contains(imagePath, "..") {
		http.Error(w, "無効なパスです", http.StatusBadRequest)
		return
	}

	// キャッシュキーを生成
	cacheKey := imagePath

	// キャッシュから画像を取得
	if data, contentType, found := h.ImageCache.Get(cacheKey); found {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600") // 1時間キャッシュ
		w.Header().Set("X-Cache", "HIT")
		w.Write(data)
		return
	}

	// キャッシュにない場合はファイルシステムから読み込み
	baseImageDir := "/app/images"
	fullPath := filepath.Join(baseImageDir, imagePath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "画像が見つかりません", http.StatusNotFound)
		return
	}

	ext := filepath.Ext(fullPath)
	var contentType string
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	default:
		contentType = "application/octet-stream"
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, "画像の読み込みに失敗しました", http.StatusInternalServerError)
		return
	}

	// キャッシュに保存（サイズ制限内の場合のみ）
	h.ImageCache.Set(cacheKey, data, contentType)

	// レスポンスヘッダーを設定
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600") // 1時間キャッシュ
	w.Header().Set("X-Cache", "MISS")
	w.Write(data)
}

// GetImageCacheStats 画像キャッシュの統計情報を取得（デバッグ用）
func (h *ProductHandler) GetImageCacheStats(w http.ResponseWriter, r *http.Request) {
	count, totalSize := h.ImageCache.GetStats()
	
	stats := map[string]interface{}{
		"cache_entries": count,
		"total_size_mb": float64(totalSize) / (1024 * 1024),
		"max_size_mb":   100.0, // 設定値
		"max_age_hours": 1.0,   // 設定値
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
