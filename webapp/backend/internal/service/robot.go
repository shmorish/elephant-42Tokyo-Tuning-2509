package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"slices"
)

type RobotService struct {
	store *repository.Store
}

func NewRobotService(store *repository.Store) *RobotService {
	return &RobotService{store: store}
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx)
			if err != nil {
				return err
			}
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				// ログ出力を削減（パフォーマンス向上）
				// log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus)
	})
}

func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	n := len(orders)
	if n == 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  0,
			Orders:      []model.Order{},
		}, nil
	}

	// 2次元DPテーブル: dp[i][w] = 最初のi個の注文で重さw以下の最大価値
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, robotCapacity+1)
	}

	// DPテーブルを埋める
	for i := 1; i <= n; i++ {
		order := orders[i-1]
		for w := 0; w <= robotCapacity; w++ {
			// 注文iを選ばない場合
			dp[i][w] = dp[i-1][w]

			// 注文iを選ぶ場合（重さ制限を満たす場合のみ）
			if order.Weight <= w {
				selectValue := dp[i-1][w-order.Weight] + order.Value
				if selectValue > dp[i][w] {
					dp[i][w] = selectValue
				}
			}
		}

		// コンテキストキャンセルチェック
		if i%100 == 0 {
			select {
			case <-ctx.Done():
				return model.DeliveryPlan{}, ctx.Err()
			default:
			}
		}
	}

	bestValue := dp[n][robotCapacity]

	// 最適解を復元
	var selectedOrders []model.Order
	w := robotCapacity
	for i := n; i > 0 && w > 0; i-- {
		select {
		case <-ctx.Done():
			return model.DeliveryPlan{}, ctx.Err()
		default:
		}

		order := orders[i-1]

		// 安全な順序で条件評価
		if w >= order.Weight && dp[i-1][w-order.Weight]+order.Value == dp[i][w] {
			selectedOrders = append(selectedOrders, order)
			w -= order.Weight
		}
	}

	// 注文の順序を元に戻す（復元は逆順で行われているため）
	slices.Reverse(selectedOrders)

	// 総重量を計算
	var totalWeight int
	for _, order := range selectedOrders {
		totalWeight += order.Weight
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      selectedOrders,
	}, nil
}
