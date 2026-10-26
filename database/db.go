package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect() (*pgxpool.Pool, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	config.MaxConnLifetime = time.Hour

	db, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	migrations := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT`,
		`ALTER TABLE users DROP COLUMN IF EXISTS pass_hash`,
		`CREATE TABLE IF NOT EXISTS orders (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT NOT NULL REFERENCES users(id),
			shop_id BIGINT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS order_items (
			id BIGSERIAL PRIMARY KEY,
			order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			product_id BIGINT REFERENCES products(id) ON DELETE SET NULL,
			product_name TEXT NOT NULL,
			quantity INTEGER NOT NULL CHECK (quantity > 0),
			unit_price NUMERIC(12, 2) NOT NULL CHECK (unit_price >= 0)
		)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS size TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS image_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ALTER COLUMN name TYPE VARCHAR(40)`,
		`DO $$
		DECLARE constraint_name TEXT;
		BEGIN
			FOR constraint_name IN
				SELECT c.conname
				FROM pg_constraint c
				JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
				WHERE c.conrelid = 'shops'::regclass AND c.contype = 'u'
				GROUP BY c.conname
				HAVING array_agg(a.attname ORDER BY a.attname) = ARRAY['owner_id']::name[]
			LOOP
				EXECUTE format('ALTER TABLE shops DROP CONSTRAINT %I', constraint_name);
			END LOOP;
		END $$`,
		`DO $$
		DECLARE
			index_schema TEXT;
			index_name TEXT;
		BEGIN
			FOR index_schema, index_name IN
				SELECT ns.nspname, idx.relname
				FROM pg_index i
			JOIN pg_class tbl ON tbl.oid = i.indrelid
			JOIN pg_class idx ON idx.oid = i.indexrelid
			JOIN pg_namespace ns ON ns.oid = idx.relnamespace
			JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = i.indkey[0]
			WHERE tbl.oid = 'shops'::regclass AND i.indisunique AND NOT i.indisprimary
			  AND i.indnkeyatts = 1 AND a.attname = 'owner_id'
			  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid)
			LOOP
				EXECUTE format('DROP INDEX %I.%I', index_schema, index_name);
			END LOOP;
		END $$`,
	}
	for _, migration := range migrations {
		if _, err := db.Exec(ctx, migration); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply database migrations: %w", err)
		}
	}

	return db, nil
}
