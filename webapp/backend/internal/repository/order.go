package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES (?, ?, 'shipping', NOW())`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// 複数の注文を一括で作成し、生成された注文IDのリストを返す
func (r *OrderRepository) CreateBulk(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	if len(items) == 0 {
		return []string{}, nil
	}

	// 数量分の値を準備
	var values []string
	var args []interface{}

	for _, item := range items {
		for i := 0; i < item.Quantity; i++ {
			values = append(values, "(?, ?, 'shipping', NOW())")
			args = append(args, userID, item.ProductID)
		}
	}

	if len(values) == 0 {
		return []string{}, nil
	}

	// バルクINSERTクエリを構築
	query := fmt.Sprintf("INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES %s",
		strings.Join(values, ", "))

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	// 最初のIDを取得
	firstID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 影響を受けた行数を取得
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	// IDのリストを生成
	orderIDs := make([]string, rowsAffected)
	for i := int64(0); i < rowsAffected; i++ {
		orderIDs[i] = fmt.Sprintf("%d", firstID+i)
	}

	return orderIDs, nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
        ORDER BY o.created_at ASC
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	var searchCondition string
	var searchArgs []interface{}
	args := []interface{}{userID}

	if req.Search != "" {
		if req.Type == "prefix" {
			// 前方一致検索（インデックス活用）
			searchCondition = "AND p.name LIKE ?"
			searchArgs = append(searchArgs, req.Search+"%")
		} else {
			// 部分一致検索（LIKE検索使用）
			searchCondition = "AND p.name LIKE ?"
			searchArgs = append(searchArgs, "%"+req.Search+"%")
		}
		args = append(args, searchArgs...)
	}

	var orderByClause string
	switch req.SortField {
	case "product_name":
		orderByClause = "ORDER BY p.name"
	case "created_at":
		orderByClause = "ORDER BY o.created_at"
	case "shipped_status":
		orderByClause = "ORDER BY o.shipped_status"
	case "arrived_at":
		orderByClause = "ORDER BY o.arrived_at"
	case "order_id":
		fallthrough
	default:
		orderByClause = "ORDER BY o.order_id"
	}

	if strings.ToUpper(req.SortOrder) == "DESC" {
		orderByClause += " DESC"
	} else {
		orderByClause += " ASC"
	}

	// 1回のクエリでデータとカウントの両方を取得（ウィンドウ関数使用）
	query := fmt.Sprintf(`
		SELECT
			o.order_id,
			o.product_id,
			p.name as product_name,
			o.shipped_status,
			o.created_at,
			o.arrived_at,
			COUNT(*) OVER() as total_count
		FROM orders o
		JOIN products p ON o.product_id = p.product_id
		WHERE o.user_id = ?
		%s
		%s
		LIMIT ? OFFSET ?
	`, searchCondition, orderByClause)

	args = append(args, req.PageSize, req.Offset)

	type orderRowWithCount struct {
		OrderID       int          `db:"order_id"`
		ProductID     int          `db:"product_id"`
		ProductName   string       `db:"product_name"`
		ShippedStatus string       `db:"shipped_status"`
		CreatedAt     sql.NullTime `db:"created_at"`
		ArrivedAt     sql.NullTime `db:"arrived_at"`
		TotalCount    int          `db:"total_count"`
	}

	var ordersRaw []orderRowWithCount
	err := r.db.SelectContext(ctx, &ordersRaw, query, args...)
	if err != nil {
		return nil, 0, err
	}

	if len(ordersRaw) == 0 {
		return []model.Order{}, 0, nil
	}

	// 最初の行からtotal_countを取得
	total := ordersRaw[0].TotalCount

	orders := make([]model.Order, len(ordersRaw))
	for i, o := range ordersRaw {
		orders[i] = model.Order{
			OrderID:       int64(o.OrderID),
			ProductID:     o.ProductID,
			ProductName:   o.ProductName,
			ShippedStatus: o.ShippedStatus,
			CreatedAt:     o.CreatedAt.Time,
			ArrivedAt:     o.ArrivedAt,
		}
	}

	return orders, total, nil
}
