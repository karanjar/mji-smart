// internal/repository/report_repository.go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/karanjar/mji-smart/internal/cache"
	"github.com/karanjar/mji-smart/internal/models"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// ReportRepository handles report	database operations with caching
type ReportRepository struct {
	db     *sql.DB
	cache  cache.QueryCache
	logger *zap.Logger
}

// NewReportRepository creates a new repository instance
// cache parameter can be nil - if nil, caching is disabled
func NewReportRepository(db *sql.DB, cache cache.QueryCache, logger *zap.Logger) *ReportRepository {
	return &ReportRepository{
		db:     db,
		cache:  cache,
		logger: logger,
	}
}

// GetByID retrieves a report by ID with caching
func (r *ReportRepository) GetByID(ctx context.Context, id string) (*models.Report, error) {
	cacheKey := fmt.Sprintf("report:%s", id)

	// If cache is enabled, try to get from cache first
	if r.cache != nil {
		var report models.Report
		err := r.cache.GetOrSet(ctx, cacheKey, &report, 5*time.Minute, func() (interface{}, error) {
			return r.fetchReportByID(ctx, id)
		})

		if err != nil {
			return nil, err
		}

		return &report, nil
	}

	// Cache disabled, fetch directly
	return r.fetchReportByID(ctx, id)
}

// fetchReportByID is the actual database query
func (r *ReportRepository) fetchReportByID(ctx context.Context, id string) (*models.Report, error) {
	query := `
        SELECT id, user_id, category, ST_AsText(location) as coords, address,
               image_url, image_thumb_url, description, status, severity,
               ai_confidence, metadata, created_at, updated_at, resolved_at
        FROM reports
        WHERE id = $1 AND deleted_at IS NULL
    `

	var report models.Report
	var coords string
	var resolvedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&report.ID, &report.UserID, &report.Category, &coords, &report.Address,
		&report.ImageURL, &report.ImageThumbURL, &report.Description, &report.Status,
		&report.Severity, &report.AiConfidence, &report.Metadata, &report.CreatedAt,
		&report.UpdatedAt, &resolvedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to fetch report: %w", err)
	}

	report.Coords = coords
	if resolvedAt.Valid {
		report.ResolvedAt = resolvedAt
	}

	return &report, nil
}

// GetNearbyReports retrieves reports near a location with caching
func (r *ReportRepository) GetNearbyReports(ctx context.Context, lat, lng float64, radiusMeters int, limit int) ([]*models.Report, error) {
	cacheKey := fmt.Sprintf("nearby:%.6f:%.6f:%d:%d", lat, lng, radiusMeters, limit)

	if r.cache != nil {
		var reports []*models.Report
		err := r.cache.GetOrSet(ctx, cacheKey, &reports, 2*time.Minute, func() (interface{}, error) {
			return r.fetchNearbyReports(ctx, lat, lng, radiusMeters, limit)
		})

		if err != nil {
			return nil, err
		}

		return reports, nil
	}

	return r.fetchNearbyReports(ctx, lat, lng, radiusMeters, limit)
}

// fetchNearbyReports is the actual database query
func (r *ReportRepository) fetchNearbyReports(ctx context.Context, lat, lng float64, radiusMeters int, limit int) ([]*models.Report, error) {
	query := `
        SELECT id, user_id, category, ST_AsText(location) as coords, address,
               image_thumb_url, description, status, severity, created_at,
               ST_DistanceSphere(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)) as distance
        FROM reports
        WHERE ST_DWithinSphere(location, ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)
            AND deleted_at IS NULL
            AND status != 'rejected'
        ORDER BY distance ASC, severity DESC
        LIMIT $4
    `

	rows, err := r.db.QueryContext(ctx, query, lng, lat, radiusMeters, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nearby reports: %w", err)
	}
	defer rows.Close()

	var reports []*models.Report
	for rows.Next() {
		var report models.Report
		var coords string
		var distance float64

		err := rows.Scan(
			&report.ID, &report.UserID, &report.Category, &coords, &report.Address,
			&report.ImageThumbURL, &report.Description, &report.Status,
			&report.Severity, &report.CreatedAt, &distance,
		)

		if err != nil {
			r.logger.Error("Failed to scan report", zap.Error(err))
			continue
		}

		report.Coords = coords
		reports = append(reports, &report)
	}

	return reports, rows.Err()
}

// Create creates a new report and invalidates relevant caches
func (r *ReportRepository) Create(ctx context.Context, report *models.Report) error {
	query := `
        INSERT INTO reports (id, user_id, category, location, address, 
                            image_url, image_thumb_url, description, 
                            status, metadata)
        VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), 
                $6, $7, $8, $9, $10, $11)
        RETURNING created_at
    `

	// Parse coordinates
	var lat, lng float64
	fmt.Sscanf(report.Coords, "%f,%f", &lat, &lng)

	err := r.db.QueryRowContext(ctx, query,
		report.ID, report.UserID, report.Category, lng, lat,
		report.Address, report.ImageURL, report.ImageThumbURL,
		report.Description, report.Status, report.Metadata,
	).Scan(&report.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}

	// Invalidate nearby caches after creating new report
	if r.cache != nil {
		go func() {
			invalidateCtx := context.Background()
			if err := r.cache.DeletePattern(invalidateCtx, "nearby:*"); err != nil {
				r.logger.Error("Failed to invalidate nearby cache", zap.Error(err))
			}
		}()
	}

	return nil
}

// UpdateStatus updates report status and invalidates caches
func (r *ReportRepository) UpdateStatus(ctx context.Context, id string, status string, adminID string) error {
	query := `
        UPDATE reports
        SET status = $2, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1 AND deleted_at IS NULL
        RETURNING updated_at
    `

	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, query, id, status).Scan(&updatedAt)
	if err != nil {
		return fmt.Errorf("failed to update report status: %w", err)
	}

	// Invalidate caches for this report
	if r.cache != nil {
		go func() {
			invalidateCtx := context.Background()
			cacheKey := fmt.Sprintf("report:%s", id)
			if err := r.cache.Delete(invalidateCtx, cacheKey); err != nil {
				r.logger.Error("Failed to invalidate report cache",
					zap.String("report_id", id),
					zap.Error(err))
			}

			// Also invalidate nearby queries since status changed
			if err := r.cache.DeletePattern(invalidateCtx, "nearby:*"); err != nil {
				r.logger.Error("Failed to invalidate nearby cache", zap.Error(err))
			}
		}()
	}

	return nil
}

// GetUserReports retrieves reports by user with pagination
func (r *ReportRepository) GetUserReports(ctx context.Context, userID string, limit, offset int) ([]*models.Report, error) {
	cacheKey := fmt.Sprintf("user_reports:%s:%d:%d", userID, limit, offset)

	if r.cache != nil {
		var reports []*models.Report
		err := r.cache.GetOrSet(ctx, cacheKey, &reports, 3*time.Minute, func() (interface{}, error) {
			return r.fetchUserReports(ctx, userID, limit, offset)
		})

		if err != nil {
			return nil, err
		}

		return reports, nil
	}

	return r.fetchUserReports(ctx, userID, limit, offset)
}

// fetchUserReports is the actual database query
func (r *ReportRepository) fetchUserReports(ctx context.Context, userID string, limit, offset int) ([]*models.Report, error) {
	query := `
        SELECT id, category, ST_AsText(location) as coords, address,
               image_thumb_url, description, status, severity, created_at, updated_at
        FROM reports
        WHERE user_id = $1 AND deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
    `

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user reports: %w", err)
	}
	defer rows.Close()

	var reports []*models.Report
	for rows.Next() {
		var report models.Report
		var coords string

		err := rows.Scan(
			&report.ID, &report.Category, &coords, &report.Address,
			&report.ImageThumbURL, &report.Description, &report.Status,
			&report.Severity, &report.CreatedAt, &report.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan report: %w", err)
		}

		report.Coords = coords
		reports = append(reports, &report)
	}

	return reports, rows.Err()
}

// GetStatistics retrieves report statistics with caching
func (r *ReportRepository) GetStatistics(ctx context.Context, days int) (*ReportStatistics, error) {
	cacheKey := fmt.Sprintf("stats:%d", days)

	if r.cache != nil {
		var stats ReportStatistics
		err := r.cache.GetOrSet(ctx, cacheKey, &stats, 10*time.Minute, func() (interface{}, error) {
			return r.fetchStatistics(ctx, days)
		})

		if err != nil {
			return nil, err
		}

		return &stats, nil
	}

	return r.fetchStatistics(ctx, days)
}

// fetchStatistics is the actual database query
func (r *ReportRepository) fetchStatistics(ctx context.Context, days int) (*ReportStatistics, error) {
	query := `
        SELECT 
            COUNT(*) as total_reports,
            COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved_reports,
            COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_reports,
            AVG(severity) as avg_severity,
            AVG(EXTRACT(EPOCH FROM (resolved_at - created_at))/60) as avg_resolution_minutes
        FROM reports
        WHERE created_at > CURRENT_TIMESTAMP - ($1 || ' days')::INTERVAL
            AND deleted_at IS NULL
    `

	var stats ReportStatistics
	err := r.db.QueryRowContext(ctx, query, days).Scan(
		&stats.TotalReports, &stats.ResolvedReports, &stats.PendingReports,
		&stats.AvgSeverity, &stats.AvgResolutionMinutes,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch statistics: %w", err)
	}

	return &stats, nil
}

// ReportStatistics holds aggregated report data
type ReportStatistics struct {
	TotalReports         int     `json:"total_reports"`
	ResolvedReports      int     `json:"resolved_reports"`
	PendingReports       int     `json:"pending_reports"`
	AvgSeverity          float64 `json:"avg_severity"`
	AvgResolutionMinutes float64 `json:"avg_resolution_minutes"`
}

// ErrNotFound is returned when a report is not found
var ErrNotFound = fmt.Errorf("report not found")

// UpdateVerification updates a report with AI verification results
func (r *ReportRepository) UpdateVerification(ctx context.Context, reportID string, severity int, confidence float64) error {
	query := `
        UPDATE reports
        SET status = 'verified',
            severity = $2,
            ai_confidence = $3,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1 AND deleted_at IS NULL
        RETURNING updated_at
    `

	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, query, reportID, severity, confidence).Scan(&updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update verification: %w", err)
	}

	// Insert verification record
	verificationQuery := `
        INSERT INTO verifications (report_id, ai_confidence, severity_score, created_at)
        VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
    `

	_, err = r.db.ExecContext(ctx, verificationQuery, reportID, confidence, severity)
	if err != nil {
		r.logger.Warn("Failed to insert verification record",
			zap.Error(err),
			zap.String("report_id", reportID),
		)
	}

	// Invalidate cache
	if r.cache != nil {
		go func() {
			invalidateCtx := context.Background()
			cacheKey := fmt.Sprintf("report:%s", reportID)
			_ = r.cache.Delete(invalidateCtx, cacheKey)
			_ = r.cache.DeletePattern(invalidateCtx, "nearby:*")
		}()
	}

	return nil
}
