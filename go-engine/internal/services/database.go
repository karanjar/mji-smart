// internal/services/database.go
package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/karanjar/mji-smart/internal/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// InitDatabase initializes the database connection pool using config
func InitDatabase(cfg *config.DatabaseConfig, logger *zap.Logger) (*sql.DB, error) {
	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)

	// Add performance optimization parameters for production
	if cfg.MaxOpenConns > 0 {
		connStr += fmt.Sprintf(" pool_max_conns=%d", cfg.MaxOpenConns)
	}

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	logger.Info("Database initialized successfully",
		zap.String("host", cfg.Host),
		zap.String("database", cfg.Name),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
	)

	return db, nil
}

// MustInitDatabase initializes database and panics on error
func MustInitDatabase(cfg *config.DatabaseConfig, logger *zap.Logger) *sql.DB {
	db, err := InitDatabase(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	return db
}

// TestDatabaseConnection tests the database connection with a simple query
func TestDatabaseConnection(db *sql.DB, logger *zap.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database test query failed: %w", err)
	}

	logger.Debug("Database test query successful", zap.Int("result", result))
	return nil
}

// GetDatabaseStats returns current database connection pool statistics
func GetDatabaseStats(db *sql.DB) map[string]interface{} {
	stats := db.Stats()
	return map[string]interface{}{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration.String(),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}
