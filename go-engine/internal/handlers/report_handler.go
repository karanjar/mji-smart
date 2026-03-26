// internal/handlers/report_handler.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/karanjar/mji-smart/internal/models"
	"github.com/karanjar/mji-smart/internal/services"
	"go.uber.org/zap"
)

// ReportHandler handles report creation
func ReportHandler(reportService *services.ReportService, logger *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse request body
		var req CreateReportRequest
		if err := c.BodyParser(&req); err != nil {
			logger.Warn("Failed to parse report request", zap.Error(err))
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
		}

		// Validate required fields
		if err := req.Validate(); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation failed",
				"details": err.Error(),
			})
		}

		// Get user ID from context (set by auth middleware)
		userID := c.Locals("user_id")
		if userID == nil {
			// For demo, use a default user ID
			// In production, this would come from JWT
			userID = "default-user-id"
		}

		// Create report model
		report := &models.Report{
			ID:          uuid.New().String(),
			UserID:      userID.(string),
			Category:    req.Category,
			Coords:      fmt.Sprintf("%f,%f", req.Latitude, req.Longitude),
			Address:     req.Address,
			ImageURL:    req.ImageURL,
			Description: req.Description,
			Status:      "pending",
			Metadata:    models.JSONB(req.Metadata),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Create report in database
		ctx := c.Context()
		if err := reportService.CreateReport(ctx, report); err != nil {
			logger.Error("Failed to create report",
				zap.Error(err),
				zap.String("user_id", report.UserID),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create report",
			})
		}

		// Return success response
		return c.Status(fiber.StatusAccepted).JSON(CreateReportResponse{
			Message:   "Report submitted successfully",
			ReportID:  report.ID,
			Status:    report.Status,
			CreatedAt: report.CreatedAt,
		})
	}
}

// GetReportHandler retrieves a single report by ID
func GetReportHandler(reportService *services.ReportService, logger *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get report ID from URL parameter
		reportID := c.Params("id")
		if reportID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Report ID is required",
			})
		}

		// Get report from service
		ctx := c.Context()
		report, err := reportService.GetReportByID(ctx, reportID)
		if err != nil {
			if err == services.ErrReportNotFound {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "Report not found",
				})
			}
			logger.Error("Failed to get report",
				zap.Error(err),
				zap.String("report_id", reportID),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to retrieve report",
			})
		}

		// Return report
		return c.JSON(ReportResponse{
			ID:            report.ID,
			UserID:        report.UserID,
			Category:      report.Category,
			Coords:        report.Coords,
			Address:       report.Address,
			ImageURL:      report.ImageURL,
			ImageThumbURL: report.ImageThumbURL,
			Description:   report.Description,
			Status:        report.Status,
			Severity:      report.Severity,
			AiConfidence:  report.AiConfidence,
			Metadata:      report.Metadata,
			CreatedAt:     report.CreatedAt,
			UpdatedAt:     report.UpdatedAt,
			ResolvedAt:    report.ResolvedAt.Time,
		})
	}
}

// GetNearbyReportsHandler retrieves reports near a location
func GetNearbyReportsHandler(reportService *services.ReportService, logger *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse query parameters
		lat, err := parseFloatParam(c, "lat")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid latitude parameter",
			})
		}

		lng, err := parseFloatParam(c, "lng")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid longitude parameter",
			})
		}

		radius := c.QueryInt("radius", 1000) // Default 1km radius
		if radius <= 0 || radius > 10000 {   // Max 10km
			radius = 1000
		}

		limit := c.QueryInt("limit", 50)
		if limit <= 0 || limit > 100 {
			limit = 50
		}

		status := c.Query("status")
		if status != "" && !isValidStatus(status) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid status filter",
			})
		}

		// Get nearby reports
		ctx := c.Context()
		reports, total, err := reportService.GetNearbyReports(ctx, lat, lng, radius, limit, status)
		if err != nil {
			logger.Error("Failed to get nearby reports",
				zap.Error(err),
				zap.Float64("lat", lat),
				zap.Float64("lng", lng),
				zap.Int("radius", radius),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to retrieve nearby reports",
			})
		}

		// Convert to response format
		reportResponses := make([]NearbyReportResponse, len(reports))
		for i, report := range reports {
			reportResponses[i] = NearbyReportResponse{
				ID:             report.ID,
				Category:       report.Category,
				DistanceMeters: report.DistanceMeters,
				Severity:       report.Severity,
				Status:         report.Status,
				ImageThumbURL:  report.ImageThumbURL,
				Description:    truncateString(report.Description, 100),
				CreatedAt:      report.CreatedAt,
			}
		}

		// Return response with pagination info
		return c.JSON(NearbyReportsResponse{
			Reports: reportResponses,
			Total:   total,
			Limit:   limit,
			Offset:  0,
			Center: GeoPoint{
				Latitude:  lat,
				Longitude: lng,
			},
			Radius: radius,
		})
	}
}

// UpdateReportStatusHandler updates a report's status (admin only)
func UpdateReportStatusHandler(reportService *services.ReportService, logger *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		reportID := c.Params("id")
		if reportID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Report ID is required",
			})
		}

		var req UpdateStatusRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if !isValidStatus(req.Status) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid status value",
			})
		}

		// Get admin ID from context
		adminID := c.Locals("admin_id")
		if adminID == nil {
			adminID = "admin"
		}

		ctx := c.Context()
		if err := reportService.UpdateReportStatus(ctx, reportID, req.Status, adminID.(string), req.Notes); err != nil {
			if err == services.ErrReportNotFound {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "Report not found",
				})
			}
			logger.Error("Failed to update report status",
				zap.Error(err),
				zap.String("report_id", reportID),
				zap.String("status", req.Status),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update report status",
			})
		}

		return c.JSON(fiber.Map{
			"message":   "Report status updated successfully",
			"report_id": reportID,
			"status":    req.Status,
		})
	}
}

// GetStatsHandler retrieves report statistics
func GetStatsHandler(reportService *services.ReportService, sugar *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get days parameter (default 7 days)
		days := c.QueryInt("days", 7)
		if days <= 0 || days > 365 {
			days = 7
		}

		ctx := c.Context()
		stats, err := reportService.GetReportStatistics(ctx, days)
		if err != nil {
			sugar.Error("Failed to get statistics", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to retrieve statistics",
			})
		}

		return c.JSON(fiber.Map{
			"statistics":   stats,
			"period_days":  days,
			"generated_at": time.Now(),
		})
	}
}

// GetUserReportsHandler retrieves reports for the current user
func GetUserReportsHandler(reportService *services.ReportService, logger *zap.SugaredLogger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get user ID from context
		userID := c.Locals("user_id")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "User not authenticated",
			})
		}

		// Parse pagination
		limit := c.QueryInt("limit", 20)
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		offset := c.QueryInt("offset", 0)
		if offset < 0 {
			offset = 0
		}

		ctx := c.Context()
		reports, total, err := reportService.GetUserReports(ctx, userID.(string), limit, offset)
		if err != nil {
			logger.Error("Failed to get user reports",
				zap.Error(err),
				zap.String("user_id", userID.(string)),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to retrieve reports",
			})
		}

		// Convert to response format
		reportResponses := make([]UserReportResponse, len(reports))
		for i, report := range reports {
			reportResponses[i] = UserReportResponse{
				ID:          report.ID,
				Category:    report.Category,
				Description: report.Description,
				Status:      report.Status,
				Severity:    report.Severity,
				ImageURL:    report.ImageThumbURL,
				CreatedAt:   report.CreatedAt,
				UpdatedAt:   report.UpdatedAt,
			}
		}

		return c.JSON(PaginatedResponse{
			Data:   reportResponses,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}

// Request/Response structs

type CreateReportRequest struct {
	Category    string                 `json:"category" validate:"required,oneof=pothole burst_pipe illegal_dumping flooding"`
	Latitude    float64                `json:"latitude" validate:"required,latitude"`
	Longitude   float64                `json:"longitude" validate:"required,longitude"`
	Address     string                 `json:"address"`
	ImageURL    string                 `json:"image_url" validate:"required,url"`
	Description string                 `json:"description" validate:"required,min=5,max=500"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func (r *CreateReportRequest) Validate() error {
	if r.Category == "" {
		return fmt.Errorf("category is required")
	}

	validCategories := map[string]bool{
		"pothole": true, "burst_pipe": true,
		"illegal_dumping": true, "flooding": true,
	}

	if !validCategories[r.Category] {
		return fmt.Errorf("invalid category: %s", r.Category)
	}

	if r.Latitude < -90 || r.Latitude > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}

	if r.Longitude < -180 || r.Longitude > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}

	if r.ImageURL == "" {
		return fmt.Errorf("image URL is required")
	}

	if len(r.Description) < 5 {
		return fmt.Errorf("description must be at least 5 characters")
	}

	return nil
}

type CreateReportResponse struct {
	Message   string    `json:"message"`
	ReportID  string    `json:"report_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type ReportResponse struct {
	ID            string       `json:"id"`
	UserID        string       `json:"user_id"`
	Category      string       `json:"category"`
	Coords        string       `json:"coords"`
	Address       string       `json:"address"`
	ImageURL      string       `json:"image_url"`
	ImageThumbURL string       `json:"image_thumb_url"`
	Description   string       `json:"description"`
	Status        string       `json:"status"`
	Severity      int          `json:"severity"`
	AiConfidence  float64      `json:"ai_confidence"`
	Metadata      models.JSONB `json:"metadata"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ResolvedAt    time.Time    `json:"resolved_at,omitempty"`
}

type NearbyReportResponse struct {
	ID             string    `json:"id"`
	Category       string    `json:"category"`
	DistanceMeters float64   `json:"distance_meters"`
	Severity       int       `json:"severity"`
	Status         string    `json:"status"`
	ImageThumbURL  string    `json:"image_thumb_url"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

type NearbyReportsResponse struct {
	Reports []NearbyReportResponse `json:"reports"`
	Total   int                    `json:"total"`
	Limit   int                    `json:"limit"`
	Offset  int                    `json:"offset"`
	Center  GeoPoint               `json:"center"`
	Radius  int                    `json:"radius_meters"`
}

type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=pending verified rejected in_progress resolved"`
	Notes  string `json:"notes"`
}

type UserReportResponse struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Severity    int       `json:"severity"`
	ImageURL    string    `json:"image_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PaginatedResponse struct {
	Data   interface{} `json:"data"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// Helper functions

func parseFloatParam(c *fiber.Ctx, param string) (float64, error) {
	value := c.Query(param)
	if value == "" {
		return 0, fmt.Errorf("missing parameter: %s", param)
	}
	return strconv.ParseFloat(value, 64)
}

func isValidStatus(status string) bool {
	validStatuses := map[string]bool{
		"pending": true, "verified": true, "rejected": true,
		"in_progress": true, "resolved": true,
	}
	return validStatuses[status]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
