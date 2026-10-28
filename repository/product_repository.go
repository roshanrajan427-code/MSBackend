package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"mini-shop-backend/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductRepository struct {
	DB *pgxpool.Pool
}

func NewProductRepository(db *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{DB: db}
}

func (r *ProductRepository) Create(
	ctx context.Context,
	shopID int64,
	name string,
	stock int,
	price float64,
	color, size string,
) (models.Product, error) {
	var product models.Product

	query := `
		INSERT INTO products (shop_id, name, stock, price, color, size)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, shop_id, name, stock, price, color, size, COALESCE(image_url, '')
	`

	err := r.DB.QueryRow(
		ctx,
		query,
		shopID,
		name,
		stock,
		price,
		color,
		size,
	).Scan(
		&product.ID,
		&product.ShopID,
		&product.Name,
		&product.Stock,
		&product.Price,
		&product.Color,
		&product.Size,
		&product.ImageURL,
	)

	if err != nil {
		return product, err
	}

	return product, nil
}
func (r *ProductRepository) GetByShopID(
	ctx context.Context,
	shopID int64,
) ([]models.Product, error) {
	query := `
		SELECT id, shop_id, name, stock, price, color, size, COALESCE(image_url, '')
		FROM products
		WHERE shop_id = $1
		ORDER BY id ASC
	`

	rows, err := r.DB.Query(ctx, query, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []models.Product{}

	for rows.Next() {
		var product models.Product

		err := rows.Scan(
			&product.ID,
			&product.ShopID,
			&product.Name,
			&product.Stock,
			&product.Price,
			&product.Color,
			&product.Size,
			&product.ImageURL,
		)
		if err != nil {
			return nil, err
		}

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

func (r *ProductRepository) UpdateByOwner(ctx context.Context, productID, ownerID int64, name string, stock int, price float64, color, size string) (models.Product, bool, error) {
	var product models.Product
	err := r.DB.QueryRow(ctx, `
		UPDATE products p SET name = $3, stock = $4, price = $5, color = $6, size = $7
		FROM shops s
		WHERE p.id = $1 AND p.shop_id = s.id AND s.owner_id = $2
		RETURNING p.id, p.shop_id, p.name, p.stock, p.price, p.color, p.size, COALESCE(p.image_url, '')
	`, productID, ownerID, name, stock, price, color, size).Scan(&product.ID, &product.ShopID, &product.Name, &product.Stock, &product.Price, &product.Color, &product.Size, &product.ImageURL)
	if err != nil {
		if err == pgx.ErrNoRows {
			return product, false, nil
		}
		return product, false, err
	}
	return product, true, nil
}

func (r *ProductRepository) SetImageByOwner(ctx context.Context, productID, ownerID int64, imageURL string) (models.Product, bool, error) {
	var product models.Product
	err := r.DB.QueryRow(ctx, `
		UPDATE products p SET image_url = $3
		FROM shops s
		WHERE p.id = $1 AND p.shop_id = s.id AND s.owner_id = $2
		RETURNING p.id, p.shop_id, p.name, p.stock, p.price, p.color, p.size, COALESCE(p.image_url, '')
	`, productID, ownerID, imageURL).Scan(&product.ID, &product.ShopID, &product.Name, &product.Stock, &product.Price, &product.Color, &product.Size, &product.ImageURL)
	if err != nil {
		if err == pgx.ErrNoRows {
			return product, false, nil
		}
		return product, false, err
	}
	return product, true, nil
}

func (r *ProductRepository) DeleteByOwner(ctx context.Context, productID, ownerID int64) (bool, error) {
	result, err := r.DB.Exec(ctx, `
		DELETE FROM products p
		USING shops s
		WHERE p.id = $1 AND p.shop_id = s.id AND s.owner_id = $2
	`, productID, ownerID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

func (r *ProductRepository) OwnsProduct(
	ctx context.Context,
	productID, ownerID int64,
) (bool, error) {
	var owns bool

	err := r.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM products p
			JOIN shops s ON p.shop_id = s.id
			WHERE p.id = $1 AND s.owner_id = $2
		)
	`, productID, ownerID).Scan(&owns)

	if err != nil {
		return false, err
	}

	return owns, nil
}
