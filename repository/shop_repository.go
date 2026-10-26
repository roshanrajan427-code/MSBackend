package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"mini-shop-backend/models"
)

type ShopRepository struct {
	DB *pgxpool.Pool
}

func NewShopRepository(db *pgxpool.Pool) *ShopRepository {
	return &ShopRepository{DB: db}
}

func (r *ShopRepository) Create(
	ctx context.Context,
	ownerID int64,
	name string,
	category string,
	location string,
) (models.Shop, error) {
	var shop models.Shop

	query := `
		INSERT INTO shops (owner_id, name, category, location)
		VALUES ($1, $2, $3, $4)
		RETURNING id, owner_id, name, category, location
	`

	err := r.DB.QueryRow(
		ctx,
		query,
		ownerID,
		name,
		category,
		location,
	).Scan(
		&shop.ID,
		&shop.OwnerID,
		&shop.Name,
		&shop.Category,
		&shop.Location,
	)

	if err != nil {
		return shop, err
	}

	return shop, nil
}

func (r *ShopRepository) UpdateByOwner(ctx context.Context, shopID, ownerID int64, name, category, location string) (models.Shop, bool, error) {
	var shop models.Shop
	err := r.DB.QueryRow(ctx, `
		UPDATE shops SET name = $3, category = $4, location = $5
		WHERE id = $1 AND owner_id = $2
		RETURNING id, owner_id, name, category, location
	`, shopID, ownerID, name, category, location).Scan(&shop.ID, &shop.OwnerID, &shop.Name, &shop.Category, &shop.Location)
	if err != nil {
		if err == pgx.ErrNoRows {
			return shop, false, nil
		}
		return shop, false, err
	}
	return shop, true, nil
}
func (r *ShopRepository) GetAll(
	ctx context.Context,
) ([]models.Shop, error) {
	query := `
		SELECT id, owner_id, name, category, location
		FROM shops
		ORDER BY id ASC
	`

	rows, err := r.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shops := []models.Shop{}

	for rows.Next() {
		var shop models.Shop

		err := rows.Scan(
			&shop.ID,
			&shop.OwnerID,
			&shop.Name,
			&shop.Category,
			&shop.Location,
		)
		if err != nil {
			return nil, err
		}

		shops = append(shops, shop)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return shops, nil
}

func (r *ShopRepository) GetByOwner(ctx context.Context, ownerID int64) ([]models.Shop, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT id, owner_id, name, category, location
		FROM shops WHERE owner_id = $1 ORDER BY id ASC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shops := []models.Shop{}
	for rows.Next() {
		var shop models.Shop
		if err := rows.Scan(&shop.ID, &shop.OwnerID, &shop.Name, &shop.Category, &shop.Location); err != nil {
			return nil, err
		}
		shops = append(shops, shop)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shops, nil
}
func (r *ShopRepository) BelongsToOwner(
	ctx context.Context,
	shopID int64,
	ownerID int64,
) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM shops
			WHERE id = $1 AND owner_id = $2
		)
	`

	var exists bool

	err := r.DB.QueryRow(
		ctx,
		query,
		shopID,
		ownerID,
	).Scan(&exists)

	return exists, err
}
func (r *ShopRepository) DeleteByOwner(
	ctx context.Context,
	shopID int64,
	ownerID int64,
) (bool, error) {
	query := `
		DELETE FROM shops
		WHERE id = $1 AND owner_id = $2
	`

	result, err := r.DB.Exec(ctx, query, shopID, ownerID)
	if err != nil {
		return false, err
	}

	return result.RowsAffected() > 0, nil
}
