// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration
type Config struct {
	Database   DatabaseConfig
	Redis      RedisConfig
	Kafka      KafkaConfig
	Server     ServerConfig
	Auth       AuthConfig
	Storage    StorageConfig
	AI         AIConfig
	Monitoring MonitoringConfig
	Security   SecurityConfig
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

type ServerConfig struct {
	Port               string
	Environment        string
	RateLimitPerMinute int
	RateLimitBurst     int
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	BodyLimit          int
	Prefork            bool
}

type AuthConfig struct {
	JWTSecret         string
	AdminAPIKey       string
	JWTExpiryHours    int
	RefreshExpiryDays int
}

type StorageConfig struct {
	Bucket      string
	Region      string
	AccessKey   string
	SecretKey   string
	Endpoint    string
	UseSSL      bool
	MaxFileSize int64
}

type AIConfig struct {
	PythonAIURL string
	Timeout     time.Duration
	RetryCount  int
	BatchSize   int
}

type MonitoringConfig struct {
	EnableMetrics   bool
	EnableTracing   bool
	MetricsPort     int
	TracingEndpoint string
	LogLevel        string
}

type SecurityConfig struct {
	CSP            CSPConfig
	CORS           CORSConfig
	Helmet         HelmetConfig
	RateLimit      RateLimitConfig
	AllowedHosts   []string
	TrustedProxies []string
}

type CSPConfig struct {
	Enabled    bool     `json:"enabled"`
	DefaultSrc []string `json:"default_src"`
	ScriptSrc  []string `json:"script_src"`
	StyleSrc   []string `json:"style_src"`
	ImgSrc     []string `json:"img_src"`
	ConnectSrc []string `json:"connect_src"`
	FontSrc    []string `json:"font_src"`
	ObjectSrc  []string `json:"object_src"`
	MediaSrc   []string `json:"media_src"`
	FrameSrc   []string `json:"frame_src"`
	ReportURI  string   `json:"report_uri"`
	ReportOnly bool     `json:"report_only"`
}

// GetPolicyString returns the CSP policy as a string
func (c *CSPConfig) GetPolicyString() string {
	if !c.Enabled {
		return ""
	}

	var policies []string

	if len(c.DefaultSrc) > 0 {
		policies = append(policies, fmt.Sprintf("default-src %s", strings.Join(c.DefaultSrc, " ")))
	}

	if len(c.ScriptSrc) > 0 {
		policies = append(policies, fmt.Sprintf("script-src %s", strings.Join(c.ScriptSrc, " ")))
	}

	if len(c.StyleSrc) > 0 {
		policies = append(policies, fmt.Sprintf("style-src %s", strings.Join(c.StyleSrc, " ")))
	}

	if len(c.ImgSrc) > 0 {
		policies = append(policies, fmt.Sprintf("img-src %s", strings.Join(c.ImgSrc, " ")))
	}

	if len(c.ConnectSrc) > 0 {
		policies = append(policies, fmt.Sprintf("connect-src %s", strings.Join(c.ConnectSrc, " ")))
	}

	if len(c.FontSrc) > 0 {
		policies = append(policies, fmt.Sprintf("font-src %s", strings.Join(c.FontSrc, " ")))
	}

	if len(c.ObjectSrc) > 0 {
		policies = append(policies, fmt.Sprintf("object-src %s", strings.Join(c.ObjectSrc, " ")))
	}

	if len(c.MediaSrc) > 0 {
		policies = append(policies, fmt.Sprintf("media-src %s", strings.Join(c.MediaSrc, " ")))
	}

	if len(c.FrameSrc) > 0 {
		policies = append(policies, fmt.Sprintf("frame-src %s", strings.Join(c.FrameSrc, " ")))
	}

	if c.ReportURI != "" {
		if c.ReportOnly {
			policies = append(policies, fmt.Sprintf("report-uri %s", c.ReportURI))
		} else {
			policies = append(policies, fmt.Sprintf("report-uri %s", c.ReportURI))
		}
	}

	return strings.Join(policies, "; ")
}

type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

type HelmetConfig struct {
	Enabled                   bool   `json:"enabled"`
	XSSProtection             string `json:"xss_protection"`
	ContentTypeNosniff        string `json:"content_type_nosniff"`
	XFrameOptions             string `json:"x_frame_options"`
	HSTSMaxAge                int    `json:"hsts_max_age"`
	HSTSExcludeSubdomains     bool   `json:"hsts_exclude_subdomains"` // Note: Exclude, not Include
	ReferrerPolicy            string `json:"referrer_policy"`
	CSP                       string `json:"csp"`
	CSPReportOnly             string `json:"csp_report_only"`
	PermissionsPolicy         string `json:"permissions_policy"`
	CrossOriginEmbedderPolicy string `json:"cross_origin_embedder_policy"`
	CrossOriginOpenerPolicy   string `json:"cross_origin_opener_policy"`
	CrossOriginResourcePolicy string `json:"cross_origin_resource_policy"`
	OriginAgentCluster        bool   `json:"origin_agent_cluster"`
}

type RateLimitConfig struct {
	Enabled      bool `json:"enabled"`
	MaxPerMinute int  `json:"max_per_minute"`
	Burst        int  `json:"burst"`
	Expiration   int  `json:"expiration_seconds"`
}

// Load loads configuration from environment variables
func Load() *Config {
	// ... other config loading ...

	return &Config{
		// ... other configs ...
		Security: SecurityConfig{
			CSP: getCSPConfig(),
			CORS: CORSConfig{
				Enabled:          getEnvAsBool("CORS_ENABLED", true),
				AllowOrigins:     getEnvSlice("CORS_ALLOW_ORIGINS", []string{"*"}),
				AllowMethods:     getEnvSlice("CORS_ALLOW_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
				AllowHeaders:     getEnvSlice("CORS_ALLOW_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"}),
				ExposeHeaders:    getEnvSlice("CORS_EXPOSE_HEADERS", []string{"X-Request-ID"}),
				AllowCredentials: getEnvAsBool("CORS_ALLOW_CREDENTIALS", false),
				MaxAge:           getEnvAsInt("CORS_MAX_AGE", 300),
			},
			Helmet: HelmetConfig{
				Enabled:                   getEnvAsBool("HELMET_ENABLED", true),
				XSSProtection:             getEnv("HELMET_XSS_PROTECTION", "1; mode=block"),
				ContentTypeNosniff:        getEnv("HELMET_CONTENT_TYPE_NOSNIFF", "nosniff"),
				XFrameOptions:             getEnv("HELMET_X_FRAME_OPTIONS", "DENY"),
				HSTSMaxAge:                getEnvAsInt("HELMET_HSTS_MAX_AGE", 31536000),
				HSTSExcludeSubdomains:     getEnvAsBool("HELMET_HSTS_EXCLUDE_SUBDOMAINS", false),
				ReferrerPolicy:            getEnv("HELMET_REFERRER_POLICY", "strict-origin-when-cross-origin"),
				PermissionsPolicy:         getEnv("HELMET_PERMISSIONS_POLICY", "geolocation=(), microphone=(), camera=()"),
				CrossOriginEmbedderPolicy: getEnv("HELMET_CROSS_ORIGIN_EMBEDDER_POLICY", "require-corp"),
				CrossOriginOpenerPolicy:   getEnv("HELMET_CROSS_ORIGIN_OPENER_POLICY", "same-origin"),
				CrossOriginResourcePolicy: getEnv("HELMET_CROSS_ORIGIN_RESOURCE_POLICY", "same-origin"),
				OriginAgentCluster:        getEnvAsBool("HELMET_ORIGIN_AGENT_CLUSTER", true),
			},
			RateLimit: RateLimitConfig{
				Enabled:      getEnvAsBool("RATE_LIMIT_ENABLED", true),
				MaxPerMinute: getEnvAsInt("RATE_LIMIT_MAX_PER_MINUTE", 100),
				Burst:        getEnvAsInt("RATE_LIMIT_BURST", 20),
				Expiration:   getEnvAsInt("RATE_LIMIT_EXPIRATION_SECONDS", 60),
			},
			AllowedHosts:   getEnvSlice("ALLOWED_HOSTS", []string{"*"}),
			TrustedProxies: getEnvSlice("TRUSTED_PROXIES", []string{}),
		},
	}
}

// getCSPConfig loads CSP configuration from environment variables
func getCSPConfig() CSPConfig {
	return CSPConfig{
		Enabled:    getEnvAsBool("CSP_ENABLED", true),
		DefaultSrc: getEnvSlice("CSP_DEFAULT_SRC", []string{"'self'"}),
		ScriptSrc:  getEnvSlice("CSP_SCRIPT_SRC", []string{"'self'", "'unsafe-inline'", "'unsafe-eval'"}),
		StyleSrc:   getEnvSlice("CSP_STYLE_SRC", []string{"'self'", "'unsafe-inline'"}),
		ImgSrc:     getEnvSlice("CSP_IMG_SRC", []string{"'self'", "data:", "https:"}),
		ConnectSrc: getEnvSlice("CSP_CONNECT_SRC", []string{"'self'", "https://api.mji-smart.com"}),
		FontSrc:    getEnvSlice("CSP_FONT_SRC", []string{"'self'", "https://fonts.gstatic.com"}),
		ObjectSrc:  getEnvSlice("CSP_OBJECT_SRC", []string{"'none'"}),
		MediaSrc:   getEnvSlice("CSP_MEDIA_SRC", []string{"'self'"}),
		FrameSrc:   getEnvSlice("CSP_FRAME_SRC", []string{"'none'"}),
		ReportURI:  getEnv("CSP_REPORT_URI", ""),
		ReportOnly: getEnvAsBool("CSP_REPORT_ONLY", false),
	}
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsFloat64(key string, defaultValue float64) float64 {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvSlice(key string, defaultValue []string) []string {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	parts := strings.Split(valueStr, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}

// Validate checks if all required configuration is present
func (c *Config) Validate() error {
	if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "your-secret-key-change-in-production" {
		return fmt.Errorf("JWT_SECRET must be set to a secure value in production")
	}

	if c.Server.Environment == "production" {
		if c.Database.Password == "securepassword" {
			return fmt.Errorf("DATABASE_PASSWORD must be changed from default in production")
		}

		if c.Redis.Password == "" {
			return fmt.Errorf("REDIS_PASSWORD must be set in production")
		}

		if c.Security.CSP.Enabled && c.Security.CSP.ReportOnly {
			// In production, we should enforce CSP, not just report
			c.Security.CSP.ReportOnly = false
		}
	}

	return nil
}

// IsProduction returns true if environment is production
func (c *Config) IsProduction() bool {
	return c.Server.Environment == "production"
}

// IsDevelopment returns true if environment is development
func (c *Config) IsDevelopment() bool {
	return c.Server.Environment == "development"
}

// IsStaging returns true if environment is staging
func (c *Config) IsStaging() bool {
	return c.Server.Environment == "staging"
}
