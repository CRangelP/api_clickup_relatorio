package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	_ "github.com/lib/pq"
)

// Config contém as configurações de conexão com o banco
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime int // in minutes
	ConnMaxIdleTime int // in minutes
}

// PoolStats contains database connection pool statistics
type PoolStats struct {
	MaxOpenConnections int   `json:"max_open_connections"`
	OpenConnections    int   `json:"open_connections"`
	InUse              int   `json:"in_use"`
	Idle               int   `json:"idle"`
	WaitCount          int64 `json:"wait_count"`
	WaitDuration       int64 `json:"wait_duration_ms"`
	MaxIdleClosed      int64 `json:"max_idle_closed"`
	MaxIdleTimeClosed  int64 `json:"max_idle_time_closed"`
	MaxLifetimeClosed  int64 `json:"max_lifetime_closed"`
}

// GetPoolStats returns current connection pool statistics
func GetPoolStats(db *sql.DB) PoolStats {
	stats := db.Stats()
	return PoolStats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration.Milliseconds(),
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}

// Connect estabelece conexão com o PostgreSQL
func Connect(cfg Config) (*sql.DB, error) {
	log := logger.Global()
	
	// Apply defaults if not set
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 10
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = 5
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = 2
	}
	
	// Constrói string de conexão
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	log.Info().
		Str("host", cfg.Host).
		Str("port", cfg.Port).
		Str("user", cfg.User).
		Str("dbname", cfg.DBName).
		Str("sslmode", cfg.SSLMode).
		Int("max_open_conns", cfg.MaxOpenConns).
		Int("max_idle_conns", cfg.MaxIdleConns).
		Int("conn_max_lifetime_min", cfg.ConnMaxLifetime).
		Int("conn_max_idle_time_min", cfg.ConnMaxIdleTime).
		Msg("Conectando ao PostgreSQL")

	// Abre conexão
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão: %w", err)
	}

	// Configura pool de conexões
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTime) * time.Minute)

	// Testa conexão
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao testar conexão: %w", err)
	}

	log.Info().
		Int("max_open_conns", cfg.MaxOpenConns).
		Int("max_idle_conns", cfg.MaxIdleConns).
		Msg("Conexão com PostgreSQL estabelecida com pool configurado")
	return db, nil
}

// Close fecha a conexão com o banco
func Close(db *sql.DB) error {
	if db != nil {
		return db.Close()
	}
	return nil
}