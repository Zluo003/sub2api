\set ON_ERROR_STOP on
\pset pager off

\if :{?apply}
\else
\set apply false
\endif
\if :{?purge_ops_logs}
\else
\set purge_ops_logs false
\endif
\if :{?vacuum_full}
\else
\set vacuum_full false
\endif
\if :{?metadata_retention_days}
\else
\set metadata_retention_days 7
\endif
\if :{?log_retention_days}
\else
\set log_retention_days 30
\endif

SELECT to_regclass('public.video_tasks') IS NOT NULL AS has_video_tasks \gset
SELECT to_regclass('public.agent_generation_quotes') IS NOT NULL AS has_agent_generation_quotes \gset
SELECT to_regclass('public.temporary_assets') IS NOT NULL AS has_temporary_assets \gset
SELECT to_regclass('public.idempotency_records') IS NOT NULL AS has_idempotency_records \gset
SELECT to_regclass('public.ops_system_logs') IS NOT NULL AS has_ops_system_logs \gset
SELECT to_regclass('public.ops_error_logs') IS NOT NULL AS has_ops_error_logs \gset
SELECT to_regclass('public.ops_system_metrics') IS NOT NULL AS has_ops_system_metrics \gset

\echo 'Database overview'
SELECT
    current_database() AS database,
    pg_size_pretty(pg_database_size(current_database())) AS total_size,
    current_timestamp AS checked_at;

\echo 'Target table sizes (table + indexes + TOAST)'
WITH target_tables(table_name) AS (
    VALUES
        ('video_tasks'),
        ('agent_generation_quotes'),
        ('temporary_assets'),
        ('idempotency_records'),
        ('ops_system_logs'),
        ('ops_error_logs'),
        ('ops_system_metrics')
)
SELECT
    table_name,
    CASE
        WHEN to_regclass(format('public.%I', table_name)) IS NULL THEN 'missing'
        ELSE pg_size_pretty(pg_total_relation_size(to_regclass(format('public.%I', table_name))))
    END AS total_size
FROM target_tables
ORDER BY table_name;

\if :has_video_tasks
\echo 'Video payload storage'
SELECT
    count(*) AS task_rows,
    count(*) FILTER (
        WHERE request_json <> '{}'::jsonb
           OR upstream_response_json <> '{}'::jsonb
    ) AS rows_with_raw_payloads,
    pg_size_pretty(COALESCE(sum(
        octet_length(request_json::text)::bigint
        + octet_length(upstream_response_json::text)::bigint
    ), 0)) AS logical_payload_size,
    pg_size_pretty(COALESCE(sum(
        pg_column_size(request_json)::bigint
        + pg_column_size(upstream_response_json)::bigint
    ), 0)) AS stored_payload_size
FROM video_tasks;
\endif

\echo 'Expired or already-deleted metadata eligible for cleanup'
\if :has_agent_generation_quotes
SELECT 'agent_generation_quotes' AS table_name, count(*) AS eligible_rows
FROM agent_generation_quotes
WHERE expires_at < NOW();
\endif
\if :has_temporary_assets
SELECT 'temporary_assets' AS table_name, count(*) AS eligible_rows
FROM temporary_assets
WHERE deleted_at IS NOT NULL
  AND deleted_at < NOW() - make_interval(days => :metadata_retention_days);
\endif
\if :has_idempotency_records
SELECT 'idempotency_records' AS table_name, count(*) AS eligible_rows
FROM idempotency_records
WHERE expires_at < NOW();
\endif

\if :purge_ops_logs
\echo 'Optional operations data eligible for retention cleanup'
\if :has_ops_system_logs
SELECT 'ops_system_logs' AS table_name, count(*) AS eligible_rows
FROM ops_system_logs
WHERE created_at < NOW() - make_interval(days => :log_retention_days);
\endif
\if :has_ops_error_logs
SELECT 'ops_error_logs' AS table_name, count(*) AS eligible_rows
FROM ops_error_logs
WHERE created_at < NOW() - make_interval(days => :log_retention_days);
\endif
\if :has_ops_system_metrics
SELECT 'ops_system_metrics' AS table_name, count(*) AS eligible_rows
FROM ops_system_metrics
WHERE created_at < NOW() - make_interval(days => :log_retention_days);
\endif
\endif

\if :apply
\echo 'Applying cleanup'
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30min';

\if :has_video_tasks
WITH changed AS (
    UPDATE video_tasks
    SET request_json = '{}'::jsonb,
        upstream_response_json = '{}'::jsonb
    WHERE request_json <> '{}'::jsonb
       OR upstream_response_json <> '{}'::jsonb
    RETURNING 1
)
SELECT count(*) AS changed_video_tasks FROM changed \gset
\echo 'video_tasks payloads cleared:' :changed_video_tasks
\endif

\if :has_agent_generation_quotes
WITH deleted AS (
    DELETE FROM agent_generation_quotes
    WHERE expires_at < NOW()
    RETURNING 1
)
SELECT count(*) AS deleted_generation_quotes FROM deleted \gset
\echo 'expired generation quotes deleted:' :deleted_generation_quotes
\endif

\if :has_temporary_assets
WITH deleted AS (
    DELETE FROM temporary_assets
    WHERE deleted_at IS NOT NULL
      AND deleted_at < NOW() - make_interval(days => :metadata_retention_days)
    RETURNING 1
)
SELECT count(*) AS deleted_temporary_asset_metadata FROM deleted \gset
\echo 'deleted temporary asset metadata removed:' :deleted_temporary_asset_metadata
\endif

\if :has_idempotency_records
WITH deleted AS (
    DELETE FROM idempotency_records
    WHERE expires_at < NOW()
    RETURNING 1
)
SELECT count(*) AS deleted_idempotency_records FROM deleted \gset
\echo 'expired idempotency records deleted:' :deleted_idempotency_records
\endif

\if :purge_ops_logs
\if :has_ops_system_logs
WITH deleted AS (
    DELETE FROM ops_system_logs
    WHERE created_at < NOW() - make_interval(days => :log_retention_days)
    RETURNING 1
)
SELECT count(*) AS deleted_ops_system_logs FROM deleted \gset
\echo 'old ops system logs deleted:' :deleted_ops_system_logs
\endif
\if :has_ops_error_logs
WITH deleted AS (
    DELETE FROM ops_error_logs
    WHERE created_at < NOW() - make_interval(days => :log_retention_days)
    RETURNING 1
)
SELECT count(*) AS deleted_ops_error_logs FROM deleted \gset
\echo 'old ops error logs deleted:' :deleted_ops_error_logs
\endif
\if :has_ops_system_metrics
WITH deleted AS (
    DELETE FROM ops_system_metrics
    WHERE created_at < NOW() - make_interval(days => :log_retention_days)
    RETURNING 1
)
SELECT count(*) AS deleted_ops_system_metrics FROM deleted \gset
\echo 'old ops system metrics deleted:' :deleted_ops_system_metrics
\endif
\endif

COMMIT;

\echo 'Refreshing planner statistics and marking dead rows reusable'
\if :has_video_tasks
VACUUM (ANALYZE) video_tasks;
\endif
\if :has_agent_generation_quotes
VACUUM (ANALYZE) agent_generation_quotes;
\endif
\if :has_temporary_assets
VACUUM (ANALYZE) temporary_assets;
\endif
\if :has_idempotency_records
VACUUM (ANALYZE) idempotency_records;
\endif
\if :purge_ops_logs
\if :has_ops_system_logs
VACUUM (ANALYZE) ops_system_logs;
\endif
\if :has_ops_error_logs
VACUUM (ANALYZE) ops_error_logs;
\endif
\if :has_ops_system_metrics
VACUUM (ANALYZE) ops_system_metrics;
\endif
\endif

\if :vacuum_full
\if :has_video_tasks
\echo 'Compacting video_tasks with an exclusive table lock'
SET lock_timeout = '5s';
SET statement_timeout = 0;
VACUUM (FULL, ANALYZE) video_tasks;
\endif
\endif
\endif
