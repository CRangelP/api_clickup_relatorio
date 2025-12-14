package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/cache"
	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
)

const (
	// Cache keys
	cacheKeyHierarchy    = "metadata:hierarchy"
	cacheKeyCustomFields = "metadata:custom_fields"
	
	// Default cache TTL
	defaultCacheTTL = 5 * time.Minute
)

// MetadataService gerencia sincronização de metadados do ClickUp
type MetadataService struct {
	metadataRepo  *repository.MetadataRepository
	configRepo    *repository.ConfigRepository
	encryptionKey []byte
	cache         *cache.Cache
}

// NewMetadataService cria um novo serviço de metadados
func NewMetadataService(metadataRepo *repository.MetadataRepository, configRepo *repository.ConfigRepository, encryptionKey string) *MetadataService {
	// Usa uma chave de 32 bytes para AES-256
	key := make([]byte, 32)
	copy(key, []byte(encryptionKey))
	
	return &MetadataService{
		metadataRepo:  metadataRepo,
		configRepo:    configRepo,
		encryptionKey: key,
		cache:         cache.NewCache(defaultCacheTTL),
	}
}

// InvalidateCache clears all cached metadata
func (s *MetadataService) InvalidateCache() {
	s.cache.Clear()
}

// GetCacheStats returns cache statistics
func (s *MetadataService) GetCacheStats() int {
	return s.cache.Size()
}

// SyncMetadata sincroniza todos os metadados do ClickUp para um usuário
func (s *MetadataService) SyncMetadata(ctx context.Context, userID, token string) error {
	log := logger.Get(ctx)
	
	log.Info().Str("user_id", userID).Msg("Iniciando sincronização de metadados")
	
	// Cria cliente ClickUp
	clickupClient := client.NewClient(token)
	
	// Valida token primeiro
	if err := clickupClient.ValidateToken(ctx); err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Token inválido")
		return fmt.Errorf("token inválido: %w", err)
	}
	
	// Salva token criptografado
	encryptedToken, err := s.encryptToken(token)
	if err != nil {
		return fmt.Errorf("erro ao criptografar token: %w", err)
	}
	
	if err := s.configRepo.UpdateClickUpToken(userID, encryptedToken); err != nil {
		return fmt.Errorf("erro ao salvar token: %w", err)
	}
	
	// Busca workspaces
	workspaces, err := clickupClient.GetWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("erro ao buscar workspaces: %w", err)
	}
	
	log.Info().Int("count", len(workspaces)).Msg("Workspaces encontrados")
	
	// Salva workspaces e busca spaces
	for _, workspace := range workspaces {
		// Salva workspace
		if err := s.metadataRepo.UpsertWorkspace(repository.Workspace{
			ID:   workspace.ID,
			Name: workspace.Name,
		}); err != nil {
			log.Error().Err(err).Str("workspace_id", workspace.ID).Msg("Erro ao salvar workspace")
			continue
		}
		
		// Busca spaces do workspace
		spaces, err := clickupClient.GetSpaces(ctx, workspace.ID)
		if err != nil {
			log.Error().Err(err).Str("workspace_id", workspace.ID).Msg("Erro ao buscar spaces")
			continue
		}
		
		log.Info().Str("workspace_id", workspace.ID).Int("count", len(spaces)).Msg("Spaces encontrados")
		
		// Salva spaces e busca folders
		for _, space := range spaces {
			// Salva space
			if err := s.metadataRepo.UpsertSpace(repository.Space{
				ID:          space.ID,
				WorkspaceID: workspace.ID,
				Name:        space.Name,
			}); err != nil {
				log.Error().Err(err).Str("space_id", space.ID).Msg("Erro ao salvar space")
				continue
			}
			
			// Busca folders do space
			folders, err := clickupClient.GetFolders(ctx, space.ID)
			if err != nil {
				log.Error().Err(err).Str("space_id", space.ID).Msg("Erro ao buscar folders")
				continue
			}
			
			log.Info().Str("space_id", space.ID).Int("count", len(folders)).Msg("Folders encontrados")
			
			// Salva folders e busca listas
			for _, folder := range folders {
				// Salva folder
				if err := s.metadataRepo.UpsertFolder(repository.Folder{
					ID:      folder.ID,
					SpaceID: space.ID,
					Name:    folder.Name,
				}); err != nil {
					log.Error().Err(err).Str("folder_id", folder.ID).Msg("Erro ao salvar folder")
					continue
				}
				
				// Busca listas do folder
				lists, err := clickupClient.GetLists(ctx, folder.ID)
				if err != nil {
					log.Error().Err(err).Str("folder_id", folder.ID).Msg("Erro ao buscar listas")
					continue
				}
				
				log.Info().Str("folder_id", folder.ID).Int("count", len(lists)).Msg("Listas encontradas")
				
				// Salva listas e busca campos personalizados
				for _, list := range lists {
					// Salva lista
					if err := s.metadataRepo.UpsertList(repository.List{
						ID:       list.ID,
						FolderID: folder.ID,
						Name:     list.Name,
					}); err != nil {
						log.Error().Err(err).Str("list_id", list.ID).Msg("Erro ao salvar lista")
						continue
					}
					
					// Busca campos personalizados da lista
					fields, err := clickupClient.GetCustomFields(ctx, list.ID)
					if err != nil {
						log.Error().Err(err).Str("list_id", list.ID).Msg("Erro ao buscar campos personalizados")
						continue
					}
					
					// Salva campos personalizados
					for _, field := range fields {
						options := make(map[string]interface{})
						
						// Converte TypeConfig para options
						if field.TypeConfig != nil {
							if field.TypeConfig.Options != nil {
								optionsList := make([]map[string]interface{}, len(field.TypeConfig.Options))
								for i, opt := range field.TypeConfig.Options {
									optionsList[i] = map[string]interface{}{
										"id":         opt.ID,
										"name":       opt.Name,
										"color":      opt.Color,
										"orderindex": opt.Orderindex,
									}
								}
								options["options"] = optionsList
							}
							
							if field.TypeConfig.Precision > 0 {
								options["precision"] = field.TypeConfig.Precision
							}
							
							if field.TypeConfig.CurrencyType != "" {
								options["currency_type"] = field.TypeConfig.CurrencyType
							}
							
							options["include_time"] = field.TypeConfig.IncludeTime
							options["is_time"] = field.TypeConfig.IsTime
						}
						
						if err := s.metadataRepo.UpsertCustomField(repository.CustomField{
							ID:      field.ID,
							Name:    field.Name,
							Type:    field.Type,
							Options: options,
						}); err != nil {
							log.Error().Err(err).Str("field_id", field.ID).Msg("Erro ao salvar campo personalizado")
							continue
						}
					}
				}
			}
		}
	}
	
	// Invalidate cache after sync
	s.InvalidateCache()
	
	log.Info().Str("user_id", userID).Msg("Sincronização de metadados concluída, cache invalidado")
	return nil
}

// GetHierarchicalData retorna dados hierárquicos para interface
func (s *MetadataService) GetHierarchicalData(ctx context.Context) (*HierarchicalData, error) {
	log := logger.Get(ctx)
	
	// Check cache first
	if cached, ok := s.cache.Get(cacheKeyHierarchy); ok {
		log.Debug().Msg("Retornando dados hierárquicos do cache")
		return cached.(*HierarchicalData), nil
	}
	
	log.Debug().Msg("Cache miss - buscando dados hierárquicos do banco")
	
	workspaces, err := s.metadataRepo.GetWorkspaces()
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar workspaces: %w", err)
	}
	
	data := &HierarchicalData{
		Workspaces: make([]WorkspaceData, len(workspaces)),
	}
	
	for i, workspace := range workspaces {
		spaces, err := s.metadataRepo.GetSpacesByWorkspace(workspace.ID)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar spaces: %w", err)
		}
		
		workspaceData := WorkspaceData{
			ID:     workspace.ID,
			Name:   workspace.Name,
			Spaces: make([]SpaceData, len(spaces)),
		}
		
		for j, space := range spaces {
			folders, err := s.metadataRepo.GetFoldersBySpace(space.ID)
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar folders: %w", err)
			}
			
			spaceData := SpaceData{
				ID:      space.ID,
				Name:    space.Name,
				Folders: make([]FolderData, len(folders)),
			}
			
			for k, folder := range folders {
				lists, err := s.metadataRepo.GetListsByFolder(folder.ID)
				if err != nil {
					return nil, fmt.Errorf("erro ao buscar listas: %w", err)
				}
				
				folderData := FolderData{
					ID:    folder.ID,
					Name:  folder.Name,
					Lists: make([]ListData, len(lists)),
				}
				
				for l, list := range lists {
					folderData.Lists[l] = ListData{
						ID:   list.ID,
						Name: list.Name,
					}
				}
				
				spaceData.Folders[k] = folderData
			}
			
			workspaceData.Spaces[j] = spaceData
		}
		
		data.Workspaces[i] = workspaceData
	}
	
	// Busca campos personalizados
	fields, err := s.metadataRepo.GetCustomFields()
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar campos personalizados: %w", err)
	}
	
	data.CustomFields = make([]CustomFieldData, len(fields))
	for i, field := range fields {
		data.CustomFields[i] = CustomFieldData{
			ID:      field.ID,
			Name:    field.Name,
			Type:    field.Type,
			Options: field.Options,
		}
	}
	
	// Store in cache
	s.cache.Set(cacheKeyHierarchy, data)
	log.Debug().Int("workspaces", len(data.Workspaces)).Int("custom_fields", len(data.CustomFields)).Msg("Dados hierárquicos armazenados no cache")
	
	return data, nil
}

// GetUserToken retorna o token descriptografado do usuário
func (s *MetadataService) GetUserToken(ctx context.Context, userID string) (string, error) {
	config, err := s.configRepo.GetUserConfig(userID)
	if err != nil {
		return "", fmt.Errorf("erro ao buscar configuração: %w", err)
	}
	
	if config == nil || config.ClickUpTokenEncrypted == "" {
		return "", fmt.Errorf("token não configurado para usuário")
	}
	
	token, err := s.decryptToken(config.ClickUpTokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("erro ao descriptografar token: %w", err)
	}
	
	return token, nil
}

// encryptToken criptografa um token usando AES
func (s *MetadataService) encryptToken(token string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	
	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptToken descriptografa um token usando AES
func (s *MetadataService) decryptToken(encryptedToken string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", err
	}
	
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	
	return string(plaintext), nil
}

// HierarchicalData representa dados hierárquicos para a interface
type HierarchicalData struct {
	Workspaces   []WorkspaceData   `json:"workspaces"`
	CustomFields []CustomFieldData `json:"custom_fields"`
}

// WorkspaceData representa dados de workspace para interface
type WorkspaceData struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Spaces []SpaceData `json:"spaces"`
}

// SpaceData representa dados de space para interface
type SpaceData struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Folders []FolderData `json:"folders"`
}

// FolderData representa dados de folder para interface
type FolderData struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Lists []ListData `json:"lists"`
}

// ListData representa dados de lista para interface
type ListData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CustomFieldData representa dados de campo personalizado para interface
type CustomFieldData struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options"`
}