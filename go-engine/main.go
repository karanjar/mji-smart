// main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	fiberRedis "github.com/gofiber/storage/redis/v3" // Alias for Fiber's Redis storage
	"github.com/karanjar/mji-smart/internal/cache"
	"github.com/karanjar/mji-smart/internal/config"
	"github.com/karanjar/mji-smart/internal/handlers"
	"github.com/karanjar/mji-smart/internal/repository"
	"github.com/karanjar/mji-smart/internal/services"
	"github.com/redis/go-redis/v9" // Alias for go-redis client
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	logger, err := initLogger(cfg.Server.Environment)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	sugar := logger.Sugar()

	// Print startup banner
	printStartupBanner(cfg, sugar)

	// Initialize database
	db, err := services.InitDatabase(&cfg.Database, logger)
	if err != nil {
		sugar.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Redis client (for cache and other operations)
	redisClient := initRedis(&cfg.Redis, sugar)
	defer func() {
		if redisClient != nil {
			redisClient.Close()
		}
	}()

	// Initialize query cache
	queryCache := initCache(redisClient, &cfg.Server, logger, sugar)

	// Initialize repository
	reportRepo := repository.NewReportRepository(db, queryCache, logger)

	// Initialize AI client
	var aiClient services.AIClient
	if cfg.AI.PythonAIURL != "" {
		aiClient = services.NewHTTPAIClient(cfg.AI.PythonAIURL, logger)
		sugar.Infof("AI client configured: %s", cfg.AI.PythonAIURL)
	} else {
		sugar.Warn("AI client not configured - verification will be disabled")
	}

	// Initialize service
	reportService := services.NewReportService(reportRepo, queryCache, logger, aiClient)

	// Initialize Fiber app with optimized config
	app := fiber.New(fiber.Config{
		Prefork:               cfg.Server.Environment == "production",
		CaseSensitive:         true,
		StrictRouting:         false,
		ServerHeader:          "Mji-Smart",
		AppName:               "Mji-Smart Engine v1.0",
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           120 * time.Second,
		BodyLimit:             10 * 1024 * 1024, // 10MB
		DisableStartupMessage: true,
		JSONEncoder:           json.Marshal,
		JSONDecoder:           json.Unmarshal,
	})

	// Setup middleware
	setupMiddleware(app, cfg, redisClient, sugar)

	// Setup routes
	setupRoutes(app, reportService, sugar)

	// Start server
	startServer(app, cfg, sugar)

	// Graceful shutdown
	gracefulShutdown(app, sugar)
}

// initLogger initializes the zap logger based on environment
func initLogger(environment string) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if environment == "production" {
		logger, err = zap.NewProduction()
	} else if environment == "staging" {
		config := zap.NewProductionConfig()
		config.Level.SetLevel(zap.InfoLevel)
		config.OutputPaths = []string{"stdout", "logs/staging.log"}
		logger, err = config.Build()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err != nil {
		return nil, err
	}

	return logger, nil
}

// printStartupBanner prints the startup banner with configuration info
func printStartupBanner(cfg *config.Config, sugar *zap.SugaredLogger) {
	sugar.Info("╔══════════════════════════════════════════════════════════╗")
	sugar.Info("║           Mji-Smart Civic Engagement Platform           ║")
	sugar.Info("║                     Engine v1.0                          ║")
	sugar.Info("╚══════════════════════════════════════════════════════════╝")
	sugar.Infof("Environment: %s", cfg.Server.Environment)
	sugar.Infof("Port: %s", cfg.Server.Port)
	sugar.Infof("Database: %s:%s/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)
	sugar.Infof("Redis: %s:%s", cfg.Redis.Host, cfg.Redis.Port)
	sugar.Infof("AI Service: %s", cfg.AI.PythonAIURL)
	sugar.Info("════════════════════════════════════════════════════════════")
}

// initRedis initializes the Redis client for go-redis/v9
func initRedis(cfg *config.RedisConfig, sugar *zap.SugaredLogger) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     100,
		MinIdleConns: 10,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		sugar.Warnf("Redis connection failed: %v - caching will be disabled", err)
		return nil
	}

	sugar.Info("Redis connected successfully")
	return client
}

// initCache initializes the query cache
func initCache(redisClient *redis.Client, serverCfg *config.ServerConfig, logger *zap.Logger, sugar *zap.SugaredLogger) cache.QueryCache {
	if redisClient != nil {
		sugar.Info("Redis cache enabled")
		return cache.NewRedisQueryCache(redisClient, logger, "mjismart")
	}

	if serverCfg.Environment == "development" {
		sugar.Info("Using in-memory cache for development")
		return cache.NewMemoryQueryCache(logger)
	}

	sugar.Warn("Cache disabled - Redis not available")
	return nil
}

// setupMiddleware configures all middleware for the Fiber app
// main.go - Updated setupMiddleware function
func setupMiddleware(app *fiber.App, cfg *config.Config, redisClient *redis.Client, sugar *zap.SugaredLogger) {
	// Recovery middleware (must be first)
	app.Use(recover.New(recover.Config{
		EnableStackTrace: cfg.Server.Environment == "development",
	}))

	// Request ID middleware
	app.Use(requestid.New(requestid.Config{
		Header: "X-Request-ID",
		Generator: func() string {
			return fmt.Sprintf("req_%d", time.Now().UnixNano())
		},
	}))

	// Logger middleware
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${error} | ${locals:requestid}\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "UTC",
	}))

	// CORS middleware (with config)
	if cfg.Security.CORS.Enabled {
		corsConfig := cors.Config{
			AllowOrigins:     strings.Join(cfg.Security.CORS.AllowOrigins, ","),
			AllowMethods:     strings.Join(cfg.Security.CORS.AllowMethods, ","),
			AllowHeaders:     strings.Join(cfg.Security.CORS.AllowHeaders, ","),
			ExposeHeaders:    strings.Join(cfg.Security.CORS.ExposeHeaders, ","),
			AllowCredentials: cfg.Security.CORS.AllowCredentials,
			MaxAge:           cfg.Security.CORS.MaxAge,
		}
		app.Use(cors.New(corsConfig))
		sugar.Info("CORS middleware configured")
	}

	// Helmet middleware for security headers (with config)
	if cfg.Security.Helmet.Enabled {
		helmetConfig := helmet.Config{
			XSSProtection:         cfg.Security.Helmet.XSSProtection,
			ContentTypeNosniff:    cfg.Security.Helmet.ContentTypeNosniff,
			XFrameOptions:         cfg.Security.Helmet.XFrameOptions,
			HSTSMaxAge:            cfg.Security.Helmet.HSTSMaxAge,
			HSTSIncludeSubdomains: cfg.Security.Helmet.HSTSIncludeSubdomains,
			HSTSPreload:           cfg.Security.Helmet.HSTSPreload,
			ReferrerPolicy:        cfg.Security.Helmet.ReferrerPolicy,
		}

		// Add CSP if enabled
		if cfg.Security.CSP.Enabled {
			cspPolicy := cfg.Security.CSP.GetPolicyString()
			if cspPolicy != "" {
				if cfg.Security.CSP.ReportOnly {
					helmetConfig.CSPReportOnly = cspPolicy
				} else {
					helmetConfig.CSP = cspPolicy
				}
				sugar.Infof("CSP policy configured: %s", cspPolicy)
			}
		}

		// Add Permissions Policy
		if cfg.Security.Helmet.PermissionsPolicy != "" {
			helmetConfig.PermissionsPolicy = cfg.Security.Helmet.PermissionsPolicy
		}

		app.Use(helmet.New(helmetConfig))
		sugar.Info("Helmet middleware configured")
	}

	// Rate limiting (if enabled and Redis available)
	if cfg.Security.RateLimit.Enabled && redisClient != nil {
		redisStorage := fiberRedis.New(fiberRedis.Config{
			Host:     cfg.Redis.Host,
			Port:     cfg.Redis.Port,
			Password: cfg.Redis.Password,
			Database: cfg.Redis.DB,
		})

		app.Use(limiter.New(limiter.Config{
			Storage:    redisStorage,
			Max:        cfg.Security.RateLimit.MaxPerMinute,
			Expiration: time.Duration(cfg.Security.RateLimit.Expiration) * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				// Rate limit by IP
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":       "Too many requests",
					"message":     fmt.Sprintf("Rate limit exceeded. Maximum %d requests per minute.", cfg.Security.RateLimit.MaxPerMinute),
					"retry_after": cfg.Security.RateLimit.Expiration,
				})
			},
		}))

		sugar.Infof("Rate limiting enabled: %d requests per minute", cfg.Security.RateLimit.MaxPerMinute)
	} else if !cfg.Security.RateLimit.Enabled {
		sugar.Info("Rate limiting disabled by configuration")
	} else {
		sugar.Warn("Rate limiting disabled - Redis not available")
	}

	// Add custom headers middleware
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Powered-By", "Mji-Smart")
		c.Set("X-API-Version", "v1")
		return c.Next()
	})

	sugar.Info("Middleware configured successfully")
}

// setupRoutes configures all API routes
func setupRoutes(app *fiber.App, reportService *services.ReportService, sugar *zap.SugaredLogger) {
	// Health check endpoints
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"service":   "mjismart-engine",
			"version":   "1.0.0",
		})
	})

	app.Get("/health/detailed", func(c *fiber.Ctx) error {
		// Add detailed health checks here
		return c.JSON(fiber.Map{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"service":   "mjismart-engine",
			"version":   "1.0.0",
			"checks": fiber.Map{
				"database": "ok",
				"redis":    "ok",
				"ai":       "ok",
			},
		})
	})

	// API v1 group
	api := app.Group("/api/v1")

	// Public routes
	reports := api.Group("/reports")
	reports.Post("/", handlers.ReportHandler(reportService, sugar))
	reports.Get("/:id", handlers.GetReportHandler(reportService, sugar))
	reports.Get("/nearby", handlers.GetNearbyReportsHandler(reportService, sugar))

	// Protected routes (add auth middleware later)
	auth := api.Group("/", func(c *fiber.Ctx) error {
		// Add authentication middleware here
		return c.Next()
	})

	auth.Get("/users/:id/reports", handlers.GetUserReportsHandler(reportService, sugar))

	// Admin routes (add admin auth middleware later)
	admin := api.Group("/admin")
	admin.Patch("/reports/:id/status", handlers.UpdateReportStatusHandler(reportService, sugar))
	admin.Get("/stats", handlers.GetStatsHandler(reportService, sugar))

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Not Found",
			"message": "The requested resource does not exist",
			"path":    c.Path(),
		})
	})

	sugar.Info("Routes configured successfully")
}

// startServer starts the Fiber server
func startServer(app *fiber.App, cfg *config.Config, sugar *zap.SugaredLogger) {
	addr := fmt.Sprintf(":%s", cfg.Server.Port)

	go func() {
		sugar.Infof("Server starting on %s", addr)
		sugar.Infof("API Documentation: http://localhost%s/docs", addr)
		sugar.Infof("Health Check: http://localhost%s/health", addr)

		if err := app.Listen(addr); err != nil {
			sugar.Fatalf("Server failed to start: %v", err)
		}
	}()
}

// gracefulShutdown handles graceful server shutdown
func gracefulShutdown(app *fiber.App, sugar *zap.SugaredLogger) {
	// Create shutdown channel
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-quit
	sugar.Info("Shutting down server...")

	// Create timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown gracefully
	if err := app.ShutdownWithContext(ctx); err != nil {
		sugar.Fatalf("Server shutdown failed: %v", err)
	}

	sugar.Info("Server stopped gracefully")
	sugar.Info("All connections closed")
}
