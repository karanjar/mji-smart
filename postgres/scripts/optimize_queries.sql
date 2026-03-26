-- postgres/scripts/optimize_queries.sql
-- Regular maintenance and optimization queries

-- Enable query logging for slow queries
ALTER SYSTEM SET log_min_duration_statement = '1000'; -- Log queries taking > 1 second
SELECT pg_reload_conf();

-- Create extension for query analysis
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Function to get query performance metrics
CREATE OR REPLACE FUNCTION get_query_performance()
RETURNS TABLE(
    query_text TEXT,
    calls BIGINT,
    total_time DOUBLE PRECISION,
    mean_time DOUBLE PRECISION,
    max_time DOUBLE PRECISION,
    rows_per_call DOUBLE PRECISION
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        query,
        calls,
        total_time,
        mean_time,
        max_time,
        rows / NULLIF(calls, 0) as rows_per_call
    FROM pg_stat_statements
    WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
    ORDER BY total_time DESC
    LIMIT 50;
END;
$$ LANGUAGE plpgsql;

-- Analyze table bloat
CREATE OR REPLACE FUNCTION get_table_bloat()
RETURNS TABLE(
    table_name TEXT,
    wasted_bytes BIGINT,
    wasted_percent NUMERIC,
    suggested_command TEXT
) AS $$
BEGIN
    RETURN QUERY
    WITH bloat_info AS (
        SELECT 
            schemaname,
            tablename,
            pg_total_relation_size(schemaname||'.'||tablename) as total_size,
            pg_table_size(schemaname||'.'||tablename) as table_size,
            pg_indexes_size(schemaname||'.'||tablename) as index_size
        FROM pg_tables
        WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
    )
    SELECT 
        tablename,
        (total_size - pg_table_size(tablename)) as wasted_bytes,
        ROUND(100.0 * (total_size - pg_table_size(tablename)) / total_size, 2) as wasted_percent,
        CASE 
            WHEN (total_size - pg_table_size(tablename)) > 1024*1024*100 
            THEN 'VACUUM FULL ' || tablename
            ELSE 'No action needed'
        END as suggested_command
    FROM bloat_info
    WHERE total_size > 1024*1024*10 -- Tables larger than 10MB
    ORDER BY wasted_bytes DESC;
END;
$$ LANGUAGE plpgsql;

-- Auto-vacuum tuning
ALTER SYSTEM SET autovacuum = 'on';
ALTER SYSTEM SET autovacuum_vacuum_scale_factor = '0.05';
ALTER SYSTEM SET autovacuum_vacuum_threshold = '1000';
ALTER SYSTEM SET autovacuum_analyze_scale_factor = '0.02';
ALTER SYSTEM SET autovacuum_analyze_threshold = '500';
ALTER SYSTEM SET autovacuum_naptime = '30s';
ALTER SYSTEM SET autovacuum_max_workers = '6';

-- Table-specific autovacuum settings for large tables
ALTER TABLE reports SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 5000,
    autovacuum_analyze_scale_factor = 0.01,
    autovacuum_analyze_threshold = 2000
);