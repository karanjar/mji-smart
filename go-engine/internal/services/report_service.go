// internal/services/report_service.go
package services

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "go.uber.org/zap"

    "mji-smart/go-engine/internal/cache"
    "mji-smart/go-engine/internal/models"
    "mji-smart/go-engine/internal/repository"
)

// ReportService handles business logic for reports
type ReportService struct {
    repo     *repository.ReportRepository
    cache    cache.QueryCache
    logger   *zap.Logger
    aiClient AIClient
}

// AIClient defines the interface for AI verification service
type AIClient interface {
    VerifyReport(ctx context.Context, report *models.Report) (*VerificationResult, error)
}

// VerificationResult contains AI verification results
type VerificationResult struct {
    Verified    bool    `json:"verified"`
    Severity    int     `json:"severity"`
    Confidence  float64 `json:"confidence"`
    Category    string  `json:"category"`
    Reason      string  `json:"reason"`
    ProcessTime int     `json:"process_time_ms"`
}

// NewReportService creates a new report service
func NewReportService(
    repo *repository.ReportRepository,
    cache cache.QueryCache,
    logger *zap.Logger,
    aiClient AIClient,
) *ReportService {
    return &ReportService{
        repo:     repo,
        cache:    cache,
        logger:   logger,
        aiClient: aiClient,
    }
}

// CreateReport creates a new report
func (s *ReportService) CreateReport(ctx context.Context, report *models.Report) error {
    // Validate report
    if err := s.validateReport(report); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Set initial values
    if report.ID == "" {
        report.ID = generateID()
    }
    report.CreatedAt = time.Now()
    report.UpdatedAt = time.Now()
    report.Status = "pending"

    // Create in repository
    if err := s.repo.Create(ctx, report); err != nil {
        return fmt.Errorf("failed to create report: %w", err)
    }

    // Trigger AI verification asynchronously
    go func() {
        verifyCtx := context.Background()
        if err := s.verifyReportAsync(verifyCtx, report); err != nil {
            s.logger.Error("AI verification failed",
                zap.Error(err),
                zap.String("report_id", report.ID),
            )
        }
    }()

    s.logger.Info("Report created",
        zap.String("report_id", report.ID),
        zap.String("user_id", report.UserID),
        zap.String("category", report.Category),
    )

    return nil
}

// GetReportByID retrieves a report by ID
func (s *ReportService) GetReportByID(ctx context.Context, id string) (*models.Report, error) {
    report, err := s.repo.GetByID(ctx, id)
    if err != nil {
        if err == repository.ErrNotFound {
            return nil, ErrReportNotFound
        }
        return nil, fmt.Errorf("failed to get report: %w", err)
    }

    return report, nil
}

// GetNearbyReports retrieves reports near a location
func (s *ReportService) GetNearbyReports(ctx context.Context, lat, lng float64, radiusMeters, limit int, status string) ([]*models.Report, int, error) {
    reports, err := s.repo.GetNearbyReports(ctx, lat, lng, radiusMeters, limit, status)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to get nearby reports: %w", err)
    }

    // For now, total is the number of reports returned
    // In production, you'd have a separate count query
    total := len(reports)

    return reports, total, nil
}

// GetUserReports retrieves reports for a specific user
func (s *ReportService) GetUserReports(ctx context.Context, userID string, limit, offset int) ([]*models.Report, int, error) {
    reports, err := s.repo.GetUserReports(ctx, userID, limit, offset)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to get user reports: %w", err)
    }

    // Get total count (simplified - you should implement a count method)
    total := len(reports)

    return reports, total, nil
}

// UpdateReportStatus updates the status of a report
func (s *ReportService) UpdateReportStatus(ctx context.Context, reportID, status, adminID, notes string) error {
    // Validate status
    if !isValidStatus(status) {
        return fmt.Errorf("invalid status: %s", status)
    }

    // Check if report exists
    _, err := s.repo.GetByID(ctx, reportID)
    if err != nil {
        if err == repository.ErrNotFound {
            return ErrReportNotFound
        }
        return fmt.Errorf("failed to get report: %w", err)
    }

    // Update status
    if err := s.repo.UpdateStatus(ctx, reportID, status, adminID, notes); err != nil {
        return fmt.Errorf("failed to update report status: %w", err)
    }

    s.logger.Info("Report status updated",
        zap.String("report_id", reportID),
        zap.String("status", status),
        zap.String("admin_id", adminID),
    )

    return nil
}

// GetReportStatistics retrieves report statistics
func (s *ReportService) GetReportStatistics(ctx context.Context, days int) (*ReportStats, error) {
    stats, err := s.repo.GetStatistics(ctx, days)
    if err != nil {
        return nil, fmt.Errorf("failed to get statistics: %w", err)
    }

    return &ReportStats{
        TotalReports:        stats.TotalReports,
        ResolvedReports:     stats.ResolvedReports,
        PendingReports:      stats.PendingReports,
        AvgSeverity:         stats.AvgSeverity,
        AvgResolutionMinutes: stats.AvgResolutionMinutes,
    }, nil
}

// verifyReportAsync triggers AI verification asynchronously
func (s *ReportService) verifyReportAsync(ctx context.Context, report *models.Report) error {
    if s.aiClient == nil {
        s.logger.Warn("AI client not configured, skipping verification",
            zap.String("report_id", report.ID),
        )
        return nil
    }

    result, err := s.aiClient.VerifyReport(ctx, report)
    if err != nil {
        return fmt.Errorf("AI verification failed: %w", err)
    }

    // Update report with verification results
    if result.Verified {
        if err := s.repo.UpdateVerification(ctx, report.ID, result.Severity, result.Confidence); err != nil {
            return fmt.Errorf("failed to update verification results: %w", err)
        }

        s.logger.Info("Report verified by AI",
            zap.String("report_id", report.ID),
            zap.Int("severity", result.Severity),
            zap.Float64("confidence", result.Confidence),
        )
    } else {
        // Mark as rejected if not verified
        if err := s.repo.UpdateStatus(ctx, report.ID, "rejected", "system", result.Reason); err != nil {
            return fmt.Errorf("failed to reject report: %w", err)
        }

        s.logger.Info("Report rejected by AI",
            zap.String("report_id", report.ID),
            zap.String("reason", result.Reason),
        )
    }

    return nil
}

// validateReport validates report data
func (s *ReportService) validateReport(report *models.Report) error {
    if report.UserID == "" {
        return fmt.Errorf("user_id is required")
    }

    if report.Category == "" {
        return fmt.Errorf("category is required")
    }

    validCategories := map[string]bool{
        "pothole": true, "burst_pipe": true,
        "illegal_dumping": true, "flooding": true,
    }

    if !validCategories[report.Category] {
        return fmt.Errorf("invalid category: %s", report.Category)
    }

    if report.ImageURL == "" {
        return fmt.Errorf("image_url is required")
    }

    if len(report.Description) < 5 {
        return fmt.Errorf("description must be at least 5 characters")
    }

    if len(report.Description) > 500 {
        return fmt.Errorf("description must not exceed 500 characters")
    }

    return nil
}

// InvalidateReportCache invalidates all caches for a report
func (s *ReportService) InvalidateReportCache(ctx context.Context, reportID string) error {
    if s.cache == nil {
        return nil
    }

    patterns := []string{
        fmt.Sprintf("report:%s", reportID),
        "nearby:*",
        "stats:*",
    }

    for _, pattern := range patterns {
        if err := s.cache.DeletePattern(ctx, pattern); err != nil {
            s.logger.Error("Failed to delete cache pattern",
                zap.String("pattern", pattern),
                zap.Error(err),
            )
        }
    }

    return nil
}

// ReportStats holds aggregated report statistics
type ReportStats struct {
    TotalReports         int     `json:"total_reports"`
    ResolvedReports      int     `json:"resolved_reports"`
    PendingReports       int     `json:"pending_reports"`
    AvgSeverity          float64 `json:"avg_severity"`
    AvgResolutionMinutes float64 `json:"avg_resolution_minutes"`
}

// Helper functions

func generateID() string {
    // Simple ID generation - in production use UUID
    return fmt.Sprintf("rep_%d", time.Now().UnixNano())
}

func isValidStatus(status string) bool {
    validStatuses := map[string]bool{
        "pending": true, "verified": true, "rejected": true,
        "in_progress": true, "resolved": true,
    }
    return validStatuses[status]
}

// Errors
var (
    ErrReportNotFound = fmt.Errorf("report not found")
)