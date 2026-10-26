package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrderStock = errors.New("one or more products are unavailable or have insufficient stock")

type OrderLineInput struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type OrderItem struct {
	ProductID   *int64  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type Order struct {
	ID           int64       `json:"id"`
	ShopID       int64       `json:"shop_id"`
	ShopName     string      `json:"shop_name"`
	CustomerName string      `json:"customer_name"`
	Status       string      `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	Items        []OrderItem `json:"items"`
}

type OrderRepository struct{ DB *pgxpool.Pool }

func NewOrderRepository(db *pgxpool.Pool) *OrderRepository { return &OrderRepository{DB: db} }

// CreateForCustomer reserves stock and creates one pending order per shop in a single transaction.
func (r *OrderRepository) CreateForCustomer(ctx context.Context, customerID int64, lines []OrderLineInput) ([]int64, error) {
	if len(lines) == 0 || len(lines) > 100 {
		return nil, errors.New("cart must contain between 1 and 100 items")
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	orderIDs := make([]int64, 0)
	orderByShop := make(map[int64]int64)
	for _, line := range lines {
		if line.ProductID <= 0 || line.Quantity < 1 || line.Quantity > 99 {
			return nil, errors.New("invalid product or quantity")
		}
		var shopID int64
		var name string
		var price float64
		err := tx.QueryRow(ctx, `
			UPDATE products p SET stock = p.stock - $2
			FROM shops s
			WHERE p.id = $1 AND p.shop_id = s.id AND p.stock >= $2
			RETURNING p.shop_id, p.name, p.price
		`, line.ProductID, line.Quantity).Scan(&shopID, &name, &price)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrOrderStock
			}
			return nil, err
		}
		orderID, exists := orderByShop[shopID]
		if !exists {
			if err := tx.QueryRow(ctx, `INSERT INTO orders (customer_id, shop_id, status) VALUES ($1, $2, 'pending') RETURNING id`, customerID, shopID).Scan(&orderID); err != nil {
				return nil, err
			}
			orderByShop[shopID] = orderID
			orderIDs = append(orderIDs, orderID)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price) VALUES ($1, $2, $3, $4, $5)`, orderID, line.ProductID, name, line.Quantity, price); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return orderIDs, nil
}

func (r *OrderRepository) ListForSeller(ctx context.Context, sellerID int64) ([]Order, error) {
	return r.listOrders(ctx, `s.owner_id`, sellerID)
}

func (r *OrderRepository) ListForCustomer(ctx context.Context, customerID int64) ([]Order, error) {
	return r.listOrders(ctx, `o.customer_id`, customerID)
}

func (r *OrderRepository) listOrders(ctx context.Context, filter string, userID int64) ([]Order, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT o.id, o.shop_id, s.name, u.name, o.status, o.created_at
		FROM orders o
		JOIN shops s ON s.id = o.shop_id
		JOIN users u ON u.id = o.customer_id
		WHERE `+filter+` = $1
		ORDER BY o.created_at DESC, o.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make([]Order, 0)
	for rows.Next() {
		var order Order
		if err := rows.Scan(&order.ID, &order.ShopID, &order.ShopName, &order.CustomerName, &order.Status, &order.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range orders {
		rows, err := r.DB.Query(ctx, `SELECT product_id, product_name, quantity, unit_price FROM order_items WHERE order_id = $1 ORDER BY id`, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = make([]OrderItem, 0)
		for rows.Next() {
			var item OrderItem
			if err := rows.Scan(&item.ProductID, &item.ProductName, &item.Quantity, &item.UnitPrice); err != nil {
				rows.Close()
				return nil, err
			}
			orders[i].Items = append(orders[i].Items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return orders, nil
}

func (r *OrderRepository) SetStatusByOwner(ctx context.Context, orderID, sellerID int64, status string) (bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var shopID int64
	err = tx.QueryRow(ctx, `
		UPDATE orders o SET status = $3
		FROM shops s
		WHERE o.id = $1 AND o.shop_id = s.id AND s.owner_id = $2 AND o.status = 'pending'
		RETURNING o.shop_id
	`, orderID, sellerID, status).Scan(&shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status == "rejected" {
		if _, err := tx.Exec(ctx, `
			UPDATE products p SET stock = p.stock + oi.quantity
			FROM (
				SELECT product_id, SUM(quantity)::INTEGER AS quantity
				FROM order_items WHERE order_id = $1 AND product_id IS NOT NULL
				GROUP BY product_id
			) oi
			WHERE oi.product_id = p.id AND p.shop_id = $2
		`, orderID, shopID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
