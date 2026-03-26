// go-engine/internal/repository/report_repository.go
package repository

import (
    "context"
    "database/sql"
    "fmt"
    "time"
    "github.com/lib/pq"
    "mjismart/go-engine/internal/models"
)

type ReportRepository struct {
    db *sql.DB
    queryCache *QueryCache
}

func NewReportRepository(db *sql.DB) *ReportRepository {
    return &ReportRepository{
        db: db,
        queryCache: NewQueryCache(1000, 5*time.Minute),
    }
}

// Optimized query with caching
func (r *ReportRepository) FindNearbyReports(ctx context.Context, lat, lng float64, radius int, status string, limit int) ([]models.Report, error) {
    // Generate cache key
    cacheKey := fmt.Sprintf("nearby:%f:%f:%d:%s:%d", lat, lng, radius, status, limit)
    
    // Try cache first
    if cached, found := r.queryCache.Get(cacheKey); found {
        return cached.([]models.Report), nil
    }
    
    // Use optimized function with prepared statement
    rows, err := r.db.QueryContext(ctx, `
        SELECT id, distance_meters, severity, status, image_thumb_url, description, created_at
        FROM find_nearby_reports_optimized($1, $2, $3, $4, $5)
    `, lat, lng, radius, status, limit)
    
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var reports []models.Report
    for rows.Next() {
        var report models.Report
        var distance float64
        
        err := rows.Scan(
            &report.ID,
            &distance,
            &report.Severity,
            &report.Status,
            &report.ImageThumbURL,
            &report.Description,
            &report.CreatedAt,
        )
        if err != nil {
            return nil, err
        }
        reports = append(reports, report)
    }
    
    // Store in cache
    r.queryCache.Set(cacheKey, reports)
    
    return reports, nil
}

// Batch insert for high throughput
func (r *ReportRepository) BatchInsertReports(ctx context.Context, reports []models.Report) error {
    // Use COPY protocol for bulk inserts
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    stmt, err := tx.PrepareContext(ctx, pq.CopyIn("reports", 
        "id", "user_id", "category", "location", "address", 
        "image_url", "description", "status", "severity"))
    if err != nil {
        return err
    }
    
    for _, report := range reports {
        _, err = stmt.ExecContext(ctx,
            report.ID,
            report.UserID,
            report.Category,
            fmt.Sprintf("POINT(%f %f)", report.Longitude, report.Latitude),
            report.Address,
            report.ImageURL,
            report.Description,
            report.Status,
            report.Severity,
        )
        if err != nil {
            return err
        }
    }
    
    _, err = stmt.ExecContext(ctx)
    if err != nil {
        return err
    }
    
    err = stmt.Close()
    if err != nil {
        return err
    }
    
    return tx.Commit()
}