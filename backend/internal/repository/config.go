package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
)

// ConfigRepository gerencia configurações de usuário no banco
type ConfigRepository struct {
	db *sql.DB
}

// NewConfigRepository cria um novo repositório de configurações
func NewConfigRepository(db *sql.DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

// UserConfig representa as configurações de um usuário
type UserConfig struct {
	UserID                string    `json:"user_id" db:"user_id"`
	ClickUpTokenEncrypted string    `json:"clickup_token_encrypted" db:"clickup_token_encrypted"`
	RateLimitPerMinute    int       `json:"rate_limit_per_minute" db:"rate_limit_per_minute"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// UpsertUserConfig insere ou atualiza configurações de usuário
func (r *ConfigRepository) UpsertUserConfig(config UserConfig) error {
	log := logger.Global()
	
	query := `
		INSERT INTO user_config (user_id, clickup_token_encrypted, rate_limit_per_minute, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			clickup_token_encrypted = EXCLUDED.clickup_token_encrypted,
			rate_limit_per_minute = EXCLUDED.rate_limit_per_minute,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, config.UserID, config.ClickUpTokenEncrypted, config.RateLimitPerMinute)
	if err != nil {
		log.Error().Err(err).Str("user_id", config.UserID).Msg("Erro ao inserir/atualizar configuração do usuário")
		return fmt.Errorf("erro ao inserir/atualizar configuração do usuário: %w", err)
	}
	
	log.Info().Str("user_id", config.UserID).Msg("Configuração do usuário atualizada")
	return nil
}

// GetUserConfig obtém configurações de um usuário
func (r *ConfigRepository) GetUserConfig(userID string) (*UserConfig, error) {
	query := `
		SELECT user_id, clickup_token_encrypted, rate_limit_per_minute, created_at, updated_at
		FROM user_config 
		WHERE user_id = $1
	`
	
	var config UserConfig
	err := r.db.QueryRow(query, userID).Scan(
		&config.UserID, 
		&config.ClickUpTokenEncrypted, 
		&config.RateLimitPerMinute, 
		&config.CreatedAt, 
		&config.UpdatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Usuário não tem configuração ainda
		}
		return nil, fmt.Errorf("erro ao buscar configuração do usuário: %w", err)
	}
	
	return &config, nil
}

// UpdateClickUpToken atualiza apenas o token do ClickUp
func (r *ConfigRepository) UpdateClickUpToken(userID, encryptedToken string) error {
	log := logger.Global()
	
	query := `
		INSERT INTO user_config (user_id, clickup_token_encrypted, rate_limit_per_minute, created_at, updated_at)
		VALUES ($1, $2, 2000, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			clickup_token_encrypted = EXCLUDED.clickup_token_encrypted,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, userID, encryptedToken)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Erro ao atualizar token do ClickUp")
		return fmt.Errorf("erro ao atualizar token do ClickUp: %w", err)
	}
	
	log.Info().Str("user_id", userID).Msg("Token do ClickUp atualizado")
	return nil
}

// UpdateRateLimit atualiza apenas o rate limit
func (r *ConfigRepository) UpdateRateLimit(userID string, rateLimit int) error {
	log := logger.Global()
	
	// Valida range do rate limit
	if rateLimit < 10 || rateLimit > 10000 {
		return fmt.Errorf("rate limit deve estar entre 10 e 10000, recebido: %d", rateLimit)
	}
	
	query := `
		INSERT INTO user_config (user_id, rate_limit_per_minute, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			rate_limit_per_minute = EXCLUDED.rate_limit_per_minute,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, userID, rateLimit)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Int("rate_limit", rateLimit).Msg("Erro ao atualizar rate limit")
		return fmt.Errorf("erro ao atualizar rate limit: %w", err)
	}
	
	log.Info().Str("user_id", userID).Int("rate_limit", rateLimit).Msg("Rate limit atualizado")
	return nil
}

// DeleteUserConfig remove configurações de um usuário
func (r *ConfigRepository) DeleteUserConfig(userID string) error {
	log := logger.Global()
	
	query := "DELETE FROM user_config WHERE user_id = $1"
	
	result, err := r.db.Exec(query, userID)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Erro ao deletar configuração do usuário")
		return fmt.Errorf("erro ao deletar configuração do usuário: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("configuração do usuário não encontrada")
	}
	
	log.Info().Str("user_id", userID).Msg("Configuração do usuário removida")
	return nil
}