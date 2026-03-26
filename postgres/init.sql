-- postgres/init.sql
-- Main initialization with all optimizations

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Performance tuning parameters (set via ALTER SYSTEM)
ALTER SYSTEM SET max_connections = '500';
ALTER SYSTEM SET shared_buffers = '2GB';
ALTER SYSTEM SET effective_cache_size = '6GB';
ALTER SYSTEM SET maintenance_work_mem = '512MB';
ALTER SYSTEM SET checkpoint_completion_target = '0.9';
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = '500';
ALTER SYSTEM SET random_page_cost = '1.1';
ALTER SYSTEM SET work_mem = '50MB';
ALTER SYSTEM SET huge_pages = 'try';
ALTER SYSTEM SET effective_io_concurrency = '200';
ALTER SYSTEM SET max_parallel_workers_per_gather = '4';
ALTER SYSTEM SET parallel_tuple_cost = '0.1';
ALTER SYSTEM SET parallel_setup_cost = '1000';

-- Reload configuration
SELECT pg_reload_conf();

-- Create schemas
CREATE SCHEMA IF NOT EXISTS mjismart;
CREATE SCHEMA IF NOT EXISTS audit;

-- Set search path
SET search_path TO mjismart, public;

-- Users table with optimized indexes
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone VARCHAR(20) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE,
    name VARCHAR(255) NOT NULL,
    location GEOMETRY(POINT, 4326),
    address TEXT,
    impact_points INTEGER DEFAULT 0,
    verification_count INTEGER DEFAULT 0,
    accuracy_score DECIMAL(3,2) DEFAULT 0,
    verified BOOLEAN DEFAULT FALSE,
    last_active TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Optimized indexes for users
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_phone ON users(phone) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_location ON users USING GIST (location) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_impact_points ON users(impact_points DESC) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_verified ON users(verified) WHERE deleted_at IS NULL;

-- Partitioned reports table for scalability
CREATE TABLE reports (
    id UUID DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    category VARCHAR(50) NOT NULL,
    location GEOMETRY(POINT, 4326) NOT NULL,
    address TEXT,
    image_url TEXT,
    image_thumb_url TEXT,
    description TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    severity INTEGER CHECK (severity BETWEEN 1 AND 5),
    ai_confidence DECIMAL(3,2),
    response_time_minutes INTEGER,
    resolved_time_minutes INTEGER,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP WITH TIME ZONE,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create partition function for automatic partition creation
CREATE OR REPLACE FUNCTION create_monthly_partition()
RETURNS trigger AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    -- Generate partition name for the month of the new record
    partition_name := 'reports_' || to_char(NEW.created_at, 'YYYY_MM');
    start_date := date_trunc('month', NEW.created_at)::DATE;
    end_date := start_date + INTERVAL '1 month';
    
    -- Check if partition exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_tables 
        WHERE tablename = partition_name 
        AND schemaname = 'mjismart'
    ) THEN
        -- Create new partition
        EXECUTE format('
            CREATE TABLE IF NOT EXISTS %I PARTITION OF reports
            FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        
        -- Create indexes on the new partition
        EXECUTE format('
            CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %I(user_id)',
            partition_name || '_user_id_idx', partition_name
        );
        
        EXECUTE format('
            CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %I(status)',
            partition_name || '_status_idx', partition_name
        );
        
        EXECUTE format('
            CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %I USING GIST (location)',
            partition_name || '_location_idx', partition_name
        );
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-create partitions
CREATE TRIGGER create_report_partition
    BEFORE INSERT ON reports
    FOR EACH ROW
    EXECUTE FUNCTION create_monthly_partition();

-- Optimized indexes for reports (on parent table)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_user_id ON reports(user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_status ON reports(status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_severity ON reports(severity);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_location ON reports USING GIST (location);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_created_at ON reports(created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_status_severity ON reports(status, severity) WHERE status IN ('pending', 'verified');

-- Partial indexes for common queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_pending ON reports(id, created_at, severity) 
WHERE status = 'pending';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_high_severity ON reports(id, created_at, location) 
WHERE severity >= 4 AND status != 'resolved';

-- Full-text search indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_description_search 
ON reports USING GIN(to_tsvector('english', coalesce(description, '')));

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reports_address_search 
ON reports USING GIN(to_tsvector('english', coalesce(address, '')));

-- Verifications table with TimescaleDB for time-series optimization
SELECT create_hypertable('verifications', 'created_at', if_not_exists => TRUE);

CREATE TABLE verifications (
    id UUID DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL,
    ai_confidence DECIMAL(3,2) NOT NULL,
    severity_score INTEGER NOT NULL,
    vision_confidence DECIMAL(3,2),
    llm_confidence DECIMAL(3,2),
    detected_objects JSONB,
    model_version VARCHAR(50),
    process_time_ms INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (report_id) REFERENCES reports(id)
);

-- Time-series specific indexes
CREATE INDEX idx_verifications_report_id ON verifications(report_id, created_at DESC);
CREATE INDEX idx_verifications_created_at ON verifications(created_at DESC);

-- Materialized view for real-time dashboard
CREATE MATERIALIZED VIEW admin_dashboard_hourly AS
SELECT 
    DATE_TRUNC('hour', created_at) as hour,
    COUNT(*) as total_reports,
    COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved_reports,
    AVG(EXTRACT(EPOCH FROM (COALESCE(resolved_at, NOW()) - created_at))/60) as avg_resolution_time,
    AVG(severity) as avg_severity,
    COUNT(DISTINCT user_id) as unique_reporters,
    COUNT(CASE WHEN severity >= 4 THEN 1 END) as high_severity_count
FROM reports
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE_TRUNC('hour', created_at)
WITH DATA;

CREATE UNIQUE INDEX idx_admin_dashboard_hourly ON admin_dashboard_hourly(hour);

-- Function to refresh materialized view
CREATE OR REPLACE FUNCTION refresh_admin_dashboard()
RETURNS TRIGGER AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY admin_dashboard_hourly;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Trigger to refresh dashboard
CREATE TRIGGER refresh_dashboard_trigger
    AFTER INSERT OR UPDATE ON reports
    FOR EACH STATEMENT
    EXECUTE FUNCTION refresh_admin_dashboard();

-- Query optimization function
CREATE OR REPLACE FUNCTION find_nearby_reports_optimized(
    p_lat DOUBLE PRECISION,
    p_lng DOUBLE PRECISION,
    p_radius_meters INTEGER,
    p_status VARCHAR DEFAULT NULL,
    p_limit INTEGER DEFAULT 50
)
RETURNS TABLE(
    id UUID,
    distance_meters DOUBLE PRECISION,
    severity INTEGER,
    status VARCHAR,
    image_thumb_url TEXT,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE
) LANGUAGE plpgsql STABLE PARALLEL SAFE AS $$
BEGIN
    RETURN QUERY
    SELECT 
        r.id,
        ST_DistanceSphere(r.location, ST_SetSRID(ST_MakePoint(p_lng, p_lat), 4326)) as distance_meters,
        r.severity,
        r.status,
        r.image_thumb_url,
        r.description,
        r.created_at
    FROM reports r
    WHERE ST_DWithinSphere(
        r.location, 
        ST_SetSRID(ST_MakePoint(p_lng, p_lat), 4326), 
        p_radius_meters
    )
    AND (p_status IS NULL OR r.status = p_status)
    AND r.deleted_at IS NULL
    ORDER BY 
        CASE WHEN r.severity >= 4 THEN 0 ELSE 1 END,
        distance_meters ASC,
        r.severity DESC
    LIMIT p_limit;
END;
$$;

-- Analyze tables for query planner
ANALYZE VERBOSE users;
ANALYZE VERBOSE reports;
ANALYZE VERBOSE verifications;