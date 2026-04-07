-- +goose Up
-- pg_partman must be installed into its own schema to use partman.* qualifiers.
CREATE SCHEMA IF NOT EXISTS partman;
CREATE EXTENSION IF NOT EXISTS pg_partman SCHEMA partman;

-- Wire pg_partman to manage weekly partitions on articles.
-- p_start_partition goes back 4 weeks so historical data can be loaded immediately.
SELECT partman.create_parent(
    p_parent_table    => 'public.articles',
    p_control         => 'published_at',
    p_interval        => '1 week',
    p_start_partition => date_trunc('week', NOW() - INTERVAL '4 weeks')::TEXT
);

-- Retain 26 weeks (6 months) of partitions. Old partitions are dropped, not just detached.
UPDATE partman.part_config
SET    retention                = '26 weeks',
       retention_keep_table     = FALSE,
       infinite_time_partitions = TRUE
WHERE  parent_table = 'public.articles';

-- +goose Down
-- partman config rows are dropped with the extension; manual partition cleanup required.
DROP EXTENSION IF EXISTS pg_partman;
DROP SCHEMA IF EXISTS partman;
