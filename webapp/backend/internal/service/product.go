package service

import (
	"context"
	"log"

	"backend/internal/model"
	"backend/internal/repository"
)

type ProductService struct {
	store *repository.Store
}

func NewProductService(store *repository.Store) *ProductService {
	return &ProductService{store: store}
}

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	var insertedOrderIDs []string

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		// 数量が0より大きいアイテムのみを処理
		var validItems []model.RequestItem
		for _, item := range items {
			if item.Quantity > 0 {
				validItems = append(validItems, item)
			}
		}
		if len(validItems) == 0 {
			return nil
		}

		// バルクINSERTで一括作成
		orderIDs, err := txStore.OrderRepo.CreateBulk(ctx, userID, validItems)
		if err != nil {
			return err
		}
		insertedOrderIDs = orderIDs
		return nil
	})

	if err != nil {
		return nil, err
	}
	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	return insertedOrderIDs, nil
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	return products, total, err
}
