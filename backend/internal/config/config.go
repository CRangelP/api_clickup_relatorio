package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// Config armazena as configurações da aplicação
type Config struct {
	TokenClickUp string
	TokenAPI     string
	Port         string
	GinMode      string
}

// ErrMissingToken indica que um token obrigatório não foi configurado
var ErrMissingToken = errors.New("token obrigatório não configurado")

// Load carrega as configurações do ambiente
func Load() (*Config, error) {
	// Tenta carregar .env de múltiplos locais
	_ = godotenv.Load()        // ./backend/.env
	_ = godotenv.Load("../.env") // ./.env (raiz do projeto)

	cfg := &Config{
		TokenClickUp: os.Getenv("TOKEN_CLICKUP"),
		TokenAPI:     os.Getenv("TOKEN_API"),
		Port:         os.Getenv("PORT"),
		GinMode:      os.Getenv("GIN_MODE"),
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

	return cfg, nil
}
