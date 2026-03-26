package services

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    _ "github.com/lib/pq"
    "go.uber.org/zap"
)

type Config struct {
    DBHost         string
    DBPort         string
    DBUser         string
    DBPassword     string
    DBName         string
    RedisHost      string
    RedisPort      string
    RedisPassword  string
    KafkaBrokers   []string
    JWTSecret      string
    AdminAPIKey    string
    Port           string
    Environment    string
    S3Bucket       string
    S3Region       string
    S3AccessKey    string
    S3SecretKey    string
    PythonAIURL    string
}

func LoadConfig() *Config {
    return &Config{
        DBHost:        getEnv("DB_HOST", "localhost"),
        DBPort:        getEnv("DB_PORT", "5432"),
        DBUser:        getEnv("DB_USER", "mjismart"),
        DBPassword:    getEnv("DB_PASSWORD", "securepassword"),
        DBName:        getEnv("DB_NAME", "mjismart"),
        RedisHost:     getEnv("REDIS_HOST", "localhost"),
        RedisPort:     getEnv("REDIS_PORT", "6379"),
        RedisPassword: getEnv("REDIS_PASSWORD", ""),
        KafkaBrokers:  getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
        JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
        AdminAPIKey:   getEnv("ADMIN_API_KEY", "admin-key-123"),
        Port:          getEnv("PORT", "3000"),
        Environment:   getEnv("ENVIRONMENT", "development"),
        S3Bucket:      getEnv("S3_BUCKET", "mjismart-reports"),
        S3Region:      getEnv("S3_REGION", "us-east-1"),
        S3AccessKey:   getEnv("S3_ACCESS_KEY", ""),
        S3SecretKey:   getEnv("S3_SECRET_KEY", ""),
        PythonAIURL:   getEnv("PYTHON_AI_URL", "http://python-ai:8000"),
    }
}

func InitDatabase(config *Config) *sql.DB {
    connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName)
    
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        panic(fmt.Sprintf("Failed to connect to database: %v", err))
    }
    
    db.SetMaxOpenConns(100)
    db.SetMaxIdleConns(10)
    db.SetConnMaxLifetime(30 * time.Minute)
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := db.PingContext(ctx); err != nil {
        panic(fmt.Sprintf("Database ping failed: %v", err))
    }
    
    return db
}