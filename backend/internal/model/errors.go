package model

import "errors"

var (
	// ErrRateLimited indica que a API do ClickUp retornou 429
	ErrRateLimited = errors.New("rate limit excedido na API do ClickUp")

	// ErrUnauthorized indica token inválido
	ErrUnauthorized = errors.New("token do ClickUp inválido ou expirado")

	// ErrNotFound indica recurso não encontrado
	ErrNotFound = errors.New("recurso não encontrado no ClickUp")

	// ErrTimeout indica timeout na requisição
	ErrTimeout = errors.New("timeout na requisição para o ClickUp")

	// ErrInvalidResponse indica resposta inválida da API
	ErrInvalidResponse = errors.New("resposta inválida da API do ClickUp")
)
