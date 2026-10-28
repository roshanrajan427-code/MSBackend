-- The baseline schema already includes the other changes that used to be
-- applied inline in database.Connect(): email, orders, order_items, and the
-- product attributes/name length. This migration contains the remaining
-- schema change: owners may have more than one shop.

DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_attribute a
          ON a.attrelid = c.conrelid
         AND a.attnum = ANY(c.conkey)
        WHERE c.conrelid = 'public.shops'::regclass
          AND c.contype = 'u'
        GROUP BY c.conname
        HAVING array_agg(a.attname ORDER BY a.attname) = ARRAY['owner_id']::name[]
    LOOP
        EXECUTE format('ALTER TABLE public.shops DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;

DO $$
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
        WHERE tbl.oid = 'public.shops'::regclass
          AND i.indisunique
          AND NOT i.indisprimary
          AND i.indnkeyatts = 1
          AND a.attname = 'owner_id'
          AND NOT EXISTS (
              SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid
          )
    LOOP
        EXECUTE format('DROP INDEX %I.%I', index_schema, index_name);
    END LOOP;
END $$;
