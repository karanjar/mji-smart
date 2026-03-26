package models

import (
    "database/sql"
    "database/sql/driver"
    "encoding/json"
    "time"
)

type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
    return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
    return json.Unmarshal(value.([]byte), &j)
}

type User struct {
    ID           string    `json:"id" db:"id"`
    Name         string    `json:"name" db:"name"`
    Phone        string    `json:"phone" db:"phone"`
    Email        string    `json:"email" db:"email"`
    Location     string    `json:"location" db:"location"`
    ImpactPoints int       `json:"impact_points" db:"impact_points"`
    Verified     bool      `json:"verified" db:"verified"`
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Report struct {
    ID           string          `json:"id" db:"id"`
    UserID       string          `json:"user_id" db:"user_id"`
    Category     string          `json:"category" db:"category"` // pothole, burst_pipe, illegal_dumping, flooding
    Coords       string          `json:"coords" db:"coords"`
    Address      string          `json:"address" db:"address"`
    ImageURL     string          `json:"image_url" db:"image_url"`
    ImageThumbURL string         `json:"image_thumb_url" db:"image_thumb_url"`
    Description  string          `json:"description" db:"description"`
    Status       string          `json:"status" db:"status"` // pending, verified, rejected, in_progress, resolved
    Severity     int             `json:"severity" db:"severity"`
    AiConfidence float64         `json:"ai_confidence" db:"ai_confidence"`
    Metadata     JSONB           `json:"metadata" db:"metadata"`
    CreatedAt    time.Time       `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
    ResolvedAt   sql.NullTime    `json:"resolved_at" db:"resolved_at"`
}

type Verification struct {
    ID           string    `json:"id" db:"id"`
    ReportID     string    `json:"report_id" db:"report_id"`
    AiConfidence float64   `json:"ai_confidence" db:"ai_confidence"`
    Severity     int       `json:"severity" db:"severity"`
    ModelVersion string    `json:"model_version" db:"model_version"`
    ProcessTime  int       `json:"process_time" db:"process_time"` // milliseconds
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type Action struct {
    ID         string         `json:"id" db:"id"`
    ReportID   string         `json:"report_id" db:"report_id"`
    AdminID    string         `json:"admin_id" db:"admin_id"`
    ActionType string         `json:"action_type" db:"action_type"` // assigned, started, completed
    Notes      string         `json:"notes" db:"notes"`
    Cost       sql.NullFloat64 `json:"cost" db:"cost"`
    CreatedAt  time.Time      `json:"created_at" db:"created_at"`
}

type Notification struct {
    ID        string    `json:"id" db:"id"`
    UserID    string    `json:"user_id" db:"user_id"`
    Type      string    `json:"type" db:"type"`
    Title     string    `json:"title" db:"title"`
    Body      string    `json:"body" db:"body"`
    Data      JSONB     `json:"data" db:"data"`
    Read      bool      `json:"read" db:"read"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type ReportWithDetails struct {
    Report
    UserName     string `json:"user_name"`
    UserPhone    string `json:"user_phone"`
    AdminName    string `json:"admin_name,omitempty"`
    ResponseTime int    `json:"response_time,omitempty"` // in minutes
}