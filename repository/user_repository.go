package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	DB *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{DB: db}
}

type User struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Mobile     string `json:"mobile_number"`
	Email      string `json:"email,omitempty"`
	Role       string `json:"role"`
	IsVerified bool   `json:"is_verified"`
}

func (r *UserRepository) CreateVerified(
	ctx context.Context,
	name string,
	mobile string,
	email string,
	role string,
) (User, error) {
	var id int64

	query := `
		INSERT INTO users (name, mobile_number, email, role, is_verified)
		VALUES ($1, $2, NULLIF($3, ''), $4, TRUE)
		RETURNING id
	`

	err := r.DB.QueryRow(
		ctx, query, name, mobile, email, role,
	).Scan(&id)
	return User{ID: id, Name: name, Mobile: mobile, Email: email, Role: role, IsVerified: true}, err
}

func (r *UserRepository) VerifyUser(
	ctx context.Context,
	userID int64,
) error {
	query := `
		UPDATE users
		SET is_verified = TRUE
		WHERE id = $1 AND is_verified = FALSE
	`

	result, err := r.DB.Exec(ctx, query, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found or already verified")
	}

	return nil
}

func (r *UserRepository) FindByMobile(ctx context.Context, mobile string) (User, error) {
	var user User

	query := `
		SELECT id, name, mobile_number, COALESCE(email, ''), role, is_verified
		FROM users
		WHERE mobile_number = $1
	`

	err := r.DB.QueryRow(ctx, query, mobile).Scan(
		&user.ID,
		&user.Name,
		&user.Mobile,
		&user.Email,
		&user.Role,
		&user.IsVerified,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return user, pgx.ErrNoRows
		}
		return user, err
	}

	return user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := r.DB.QueryRow(ctx, `SELECT id, name, mobile_number, COALESCE(email, ''), role, is_verified FROM users WHERE id = $1`, id).Scan(
		&user.ID, &user.Name, &user.Mobile, &user.Email, &user.Role, &user.IsVerified,
	)
	return user, err
}
