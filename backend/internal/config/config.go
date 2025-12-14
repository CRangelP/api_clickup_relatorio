package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config armazena as configurações da aplicação
type Config struct {
	TokenClickUp  string
	TokenAPI      string
	Port          string
	GinMode       string
	LogLevel      string
	LogJSON       bool
	EncryptionKey string
	// Database configuration
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime int // in minutes
	DBConnMaxIdleTime int // in minutes
}

// ErrMissingToken indica que um token obrigatório não foi configurado
var ErrMissingToken = errors.New("token obrigatório não configurado")

// Load carrega as configurações do ambiente
func Load() (*Config, error) {
	// Tenta carregar .env de múltiplos locais
	_ = godotenv.Load()        // ./backend/.env
	_ = godotenv.Load("../.env") // ./.env (raiz do projeto)

	cfg := &Config{
		TokenClickUp:  os.Getenv("TOKEN_CLICKUP"),
		TokenAPI:      os.Getenv("TOKEN_API"),
		Port:          os.Getenv("PORT"),
		GinMode:       os.Getenv("GIN_MODE"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		LogJSON:       os.Getenv("LOG_JSON") != "false", // default: true
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		// Database configuration
		DBHost:            os.Getenv("DB_HOST"),
		DBPort:            os.Getenv("DB_PORT"),
		DBUser:            os.Getenv("DB_USER"),
		DBPassword:        os.Getenv("DB_PASSWORD"),
		DBName:            os.Getenv("DB_NAME"),
		DBSSLMode:         os.Getenv("DB_SSLMODE"),
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 0),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 0),
		DBConnMaxLifetime: getEnvInt("DB_CONN_MAX_LIFETIME", 0),
		DBConnMaxIdleTime: getEnvInt("DB_CONN_MAX_IDLE_TIME", 0),
	}

	// Validações obrigatórias
	if cfg.TokenClickUp == "" {
		return nil, errors.New("TOKEN_CLICKUP não configurado")
	}

	if cfg.TokenAPI == "" {
		return nil, errors.New("TOKEN_API não configurado")
	}

	// Defaults
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.GinMode == "" {
		cfg.GinMode = "debug"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	// Encryption key default (should be set in production)
	if cfg.EncryptionKey == "" {
		cfg.EncryptionKey = "default-encryption-key-32bytes!!"
	}

	// Database defaults
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}
	if cfg.DBPort == "" {
		cfg.DBPort = "5432"
	}
	if cfg.DBUser == "" {
		cfg.DBUser = "postgres"
	}
	if cfg.DBName == "" {
		cfg.DBName = "clickup_updater"
	}
	if cfg.DBSSLMode == "" {
		cfg.DBSSLMode = "disable"
	}
	// Connection pool defaults
	if cfg.DBMaxOpenConns == 0 {
		cfg.DBMaxOpenConns = 25
	}
	if cfg.DBMaxIdleConns == 0 {
		cfg.DBMaxIdleConns = 10
	}
	if cfg.DBConnMaxLifetime == 0 {
		cfg.DBConnMaxLifetime = 5 // 5 minutes
	}
	if cfg.DBConnMaxIdleTime == 0 {
		cfg.DBConnMaxIdleTime = 2 // 2 minutes
	}

	return cfg, nil
}

// getEnvInt returns an integer from environment variable or default value
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
