package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/limiter"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/gofiber/fiber/v2/middleware/requestid"
    "github.com/gofiber/storage/redis/v3"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.uber.org/zap"

    "mji-smart/go-engine/internal/handlers"
    "mji-smart/go-engine/internal/middleware"
    "mji-smart/go-engine/internal/models"
    "mji-smart/go-engine/internal/services"
    "mji-smart/go-engine/internal/websocket"
)

func main() {
    // Initialize logger
    logger, _ := zap.NewProduction()
    defer logger.Sync()
    sugar := logger.Sugar()

    // Load configuration
    config := services.LoadConfig()

    // Initialize database connection pool
    db := services.InitDatabase(config)
    
    // Initialize Redis for caching and rate limiting
    redisStore := redis.New(redis.Config{
        Host:     config.RedisHost,
        Port:     config.RedisPort,
        Password: config.RedisPassword,
        Database: 0,
    })

    // Initialize Kafka producer for async processing
    kafkaProducer := services.InitKafkaProducer(config)

    // Initialize S3 client for image uploads
    s3Client := services.InitS3Client(config)

    // Create Fiber app with optimized config
    app := fiber.New(fiber.Config{
        Prefork:       true, // Enable prefork for multi-core
        CaseSensitive: true,
        StrictRouting: false,
        ServerHeader:  "Mji-Smart",
        AppName:       "Mji-Smart Engine v1.0",
        ReadTimeout:   10 * time.Second,
        WriteTimeout:  10 * time.Second,
        IdleTimeout:   120 * time.Second,
        BodyLimit:     10 * 1024 * 1024, // 10MB limit
    })

    // Middleware
    app.Use(recover.New())
    app.Use(requestid.New())
    app.Use(logger.New(logger.Config{
        Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${error}\n",
    }))
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
        AllowHeaders: "Origin, Content-Type, Accept, Authorization",
    }))
    
    // Rate limiting per IP
    app.Use(limiter.New(limiter.Config{
        Storage: redisStore,
        Max:     100,
        Expiration: 1 * time.Minute,
        KeyGenerator: func(c *fiber.Ctx) string {
            return c.IP()
        },
    }))

    // Health check endpoint
    app.Get("/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "status": "healthy",
            "timestamp": time.Now().Unix(),
        })
    })

    // Metrics endpoint for Prometheus
    app.Get("/metrics", func(c *fiber.Ctx) error {
        promhttp.Handler().ServeHTTP(c.Response().BodyWriter(), c.Context())
        return nil
    })

    // API routes
    api := app.Group("/api/v1")
    
    // Public routes
    api.Post("/reports", handlers.ReportHandler(db, redisStore, kafkaProducer, s3Client, sugar))
    api.Get("/reports/:id", handlers.GetReportHandler(db, redisStore, sugar))
    api.Get("/reports/nearby", handlers.GetNearbyReportsHandler(db, redisStore, sugar))
    
    // Protected routes (require authentication)
    auth := api.Group("/", middleware.AuthMiddleware(config.JWTSecret))
    auth.Post("/users/impact-points", handlers.UpdateImpactPointsHandler(db, sugar))
    auth.Get("/users/:id/reports", handlers.GetUserReportsHandler(db, sugar))
    
    // Admin routes
    admin := api.Group("/admin", middleware.AdminMiddleware(db, config.AdminAPIKey))
    admin.Patch("/reports/:id/status", handlers.UpdateReportStatusHandler(db, sugar))
    admin.Get("/reports/pending", handlers.GetPendingReportsHandler(db, sugar))
    admin.Get("/metrics/dashboard", handlers.GetAdminMetricsHandler(db, sugar))

    // WebSocket for real-time updates
    websocketHandler := websocket.NewHandler(db, redisStore, sugar)
    app.Get("/ws/:sub_county", websocketHandler.HandleWebSocket)

    // Start background workers
    go services.StartReportProcessor(db, redisStore, kafkaProducer, sugar)
    go services.StartNotificationWorker(db, redisStore, sugar)
    go services.StartImpactPointCalculator(db, redisStore, sugar)

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        if err := app.Listen(fmt.Sprintf(":%s", config.Port)); err != nil {
            sugar.Fatalf("Server failed to start: %v", err)
        }
    }()
    
    <-quit
    sugar.Info("Shutting down server...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := app.ShutdownWithContext(ctx); err != nil {
        sugar.Fatalf("Server shutdown failed: %v", err)
    }
    
    sugar.Info("Server stopped gracefully")
}