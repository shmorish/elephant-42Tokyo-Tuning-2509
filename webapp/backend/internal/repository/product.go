package repository

import (
	"backend/internal/model"
	"context"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧をDBレベルでページングして取得
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
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
		return nil, 0, err
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
		return nil, 0, err
	}

	return products, total, nil
}
