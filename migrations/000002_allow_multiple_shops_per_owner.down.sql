-- Restore the previous one-shop-per-owner rule. This can fail if any owner
-- has multiple shops when the rollback is run; resolve those duplicates first.
ALTER TABLE public.shops
    ADD CONSTRAINT shops_owner_id_key UNIQUE (owner_id);
