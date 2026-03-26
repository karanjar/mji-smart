// go-engine/internal/database/connection.go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

type DBConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type OptimizedDB struct {
	*sql.DB
	preparedStatements map[string]*sql.Stmt
}

func NewOptimizedDB(config *DBConfig) (*OptimizedDB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode,
	)

	// Add connection pool parameters
	connStr += " pool_max_conns=100 pool_min_conns=10"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	// Prepare frequently used statements
	preparedStmts := make(map[string]*sql.Stmt)

	stmts := map[string]string{
		"findNearbyReports": `
            SELECT id, distance_meters, severity, status, image_thumb_url, description, created_at
            FROM find_nearby_reports_optimized($1, $2, $3, $4, $5)
        `,
		"getReportByID": `
            SELECT r.*, u.name, u.phone, u.impact_points
            FROM reports r
            LEFT JOIN users u ON r.user_id = u.id
            WHERE r.id = $1 AND r.deleted_at IS NULL
        `,
		"updateReportStatus": `
            UPDATE reports 
            SET status = $2, updated_at = NOW(),
                response_time_minutes = CASE 
                    WHEN $2 = 'in_progress' AND status = 'pending' 
                    THEN EXTRACT(EPOCH FROM (NOW() - created_at))/60
                    ELSE response_time_minutes
                END
            WHERE id = $1
            RETURNING id, status, updated_at
        `,
		"getUserStats": `
            SELECT 
                COUNT(*) as total_reports,
                COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved_reports,
                AVG(severity) as avg_severity,
                SUM(impact_points) as total_impact_points
            FROM reports r
            JOIN users u ON r.user_id = u.id
            WHERE u.id = $1
            GROUP BY u.id
        `,
	}

	for name, query := range stmts {
		stmt, err := db.Prepare(query)
		if err != nil {
			log.Printf("Warning: Failed to prepare statement %s: %v", name, err)
			continue
		}
		preparedStmts[name] = stmt
	}

	return &OptimizedDB{
		DB:                 db,
		preparedStatements: preparedStmts,
	}, nil
}

func (odb *OptimizedDB) GetPreparedStmt(name string) (*sql.Stmt, error) {
	stmt, ok := odb.preparedStatements[name]
	if !ok {
		return nil, fmt.Errorf("prepared statement %s not found", name)
	}
	return stmt, nil
}

func (odb *OptimizedDB) Close() error {
	for _, stmt := range odb.preparedStatements {
		stmt.Close()
	}
	return odb.DB.Close()
}
