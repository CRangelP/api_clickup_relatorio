package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
)

// MetadataRepository gerencia operações de metadados no banco
type MetadataRepository struct {
	db *sql.DB
}

// NewMetadataRepository cria um novo repositório de metadados
func NewMetadataRepository(db *sql.DB) *MetadataRepository {
	return &MetadataRepository{db: db}
}

// Workspace representa um workspace do ClickUp
type Workspace struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Space representa um space do ClickUp
type Space struct {
	ID          string    `json:"id" db:"id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	Name        string    `json:"name" db:"name"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Folder representa um folder do ClickUp
type Folder struct {
	ID        string    `json:"id" db:"id"`
	SpaceID   string    `json:"space_id" db:"space_id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// List representa uma lista do ClickUp
type List struct {
	ID        string    `json:"id" db:"id"`
	FolderID  string    `json:"folder_id" db:"folder_id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CustomField representa um campo personalizado do ClickUp
type CustomField struct {
	ID         string                 `json:"id" db:"id"`
	Name       string                 `json:"name" db:"name"`
	Type       string                 `json:"type" db:"type"`
	Options    map[string]interface{} `json:"options" db:"options"`
	OrderIndex int                    `json:"orderindex" db:"orderindex"`
	CreatedAt  time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at" db:"updated_at"`
}

// UpsertWorkspace insere ou atualiza um workspace
func (r *MetadataRepository) UpsertWorkspace(workspace Workspace) error {
	log := logger.Global()
	
	query := `
		INSERT INTO workspaces (id, name, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, workspace.ID, workspace.Name)
	if err != nil {
		log.Error().Err(err).Str("workspace_id", workspace.ID).Msg("Erro ao inserir/atualizar workspace")
		return fmt.Errorf("erro ao inserir/atualizar workspace: %w", err)
	}
	
	return nil
}

// UpsertSpace insere ou atualiza um space
func (r *MetadataRepository) UpsertSpace(space Space) error {
	log := logger.Global()
	
	query := `
		INSERT INTO spaces (id, workspace_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			name = EXCLUDED.name,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, space.ID, space.WorkspaceID, space.Name)
	if err != nil {
		log.Error().Err(err).Str("space_id", space.ID).Msg("Erro ao inserir/atualizar space")
		return fmt.Errorf("erro ao inserir/atualizar space: %w", err)
	}
	
	return nil
}

// UpsertFolder insere ou atualiza um folder
func (r *MetadataRepository) UpsertFolder(folder Folder) error {
	log := logger.Global()
	
	query := `
		INSERT INTO folders (id, space_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			space_id = EXCLUDED.space_id,
			name = EXCLUDED.name,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, folder.ID, folder.SpaceID, folder.Name)
	if err != nil {
		log.Error().Err(err).Str("folder_id", folder.ID).Msg("Erro ao inserir/atualizar folder")
		return fmt.Errorf("erro ao inserir/atualizar folder: %w", err)
	}
	
	return nil
}

// UpsertList insere ou atualiza uma lista
func (r *MetadataRepository) UpsertList(list List) error {
	log := logger.Global()
	
	query := `
		INSERT INTO lists (id, folder_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			folder_id = EXCLUDED.folder_id,
			name = EXCLUDED.name,
			updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, list.ID, list.FolderID, list.Name)
	if err != nil {
		log.Error().Err(err).Str("list_id", list.ID).Msg("Erro ao inserir/atualizar list")
		return fmt.Errorf("erro ao inserir/atualizar list: %w", err)
	}
	
	return nil
}

// UpsertCustomField insere ou atualiza um campo personalizado
func (r *MetadataRepository) UpsertCustomField(field CustomField) error {
	log := logger.Global()
	
	// Serializa options para JSONB
	optionsJSON, err := json.Marshal(field.Options)
	if err != nil {
		return fmt.Errorf("erro ao serializar options: %w", err)
	}
	
	query := `
		INSERT INTO custom_fields (id, name, type, options, orderindex, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			type = EXCLUDED.type,
			options = EXCLUDED.options,
			orderindex = EXCLUDED.orderindex,
			updated_at = NOW()
	`
	
	_, err = r.db.Exec(query, field.ID, field.Name, field.Type, optionsJSON, field.OrderIndex)
	if err != nil {
		log.Error().Err(err).Str("field_id", field.ID).Msg("Erro ao inserir/atualizar custom field")
		return fmt.Errorf("erro ao inserir/atualizar custom field: %w", err)
	}
	
	return nil
}

// GetWorkspaces retorna todos os workspaces
func (r *MetadataRepository) GetWorkspaces() ([]Workspace, error) {
	query := "SELECT id, name, created_at, updated_at FROM workspaces ORDER BY name"
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar workspaces: %w", err)
	}
	defer rows.Close()
	
	var workspaces []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erro ao escanear workspace: %w", err)
		}
		workspaces = append(workspaces, w)
	}
	
	return workspaces, nil
}

// GetSpacesByWorkspace retorna spaces de um workspace
func (r *MetadataRepository) GetSpacesByWorkspace(workspaceID string) ([]Space, error) {
	query := `
		SELECT id, workspace_id, name, created_at, updated_at 
		FROM spaces 
		WHERE workspace_id = $1 
		ORDER BY name
	`
	
	rows, err := r.db.Query(query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar spaces: %w", err)
	}
	defer rows.Close()
	
	var spaces []Space
	for rows.Next() {
		var s Space
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erro ao escanear space: %w", err)
		}
		spaces = append(spaces, s)
	}
	
	return spaces, nil
}

// GetFoldersBySpace retorna folders de um space
func (r *MetadataRepository) GetFoldersBySpace(spaceID string) ([]Folder, error) {
	query := `
		SELECT id, space_id, name, created_at, updated_at 
		FROM folders 
		WHERE space_id = $1 
		ORDER BY name
	`
	
	rows, err := r.db.Query(query, spaceID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar folders: %w", err)
	}
	defer rows.Close()
	
	var folders []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.SpaceID, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erro ao escanear folder: %w", err)
		}
		folders = append(folders, f)
	}
	
	return folders, nil
}

// GetListsByFolder retorna listas de um folder
func (r *MetadataRepository) GetListsByFolder(folderID string) ([]List, error) {
	query := `
		SELECT id, folder_id, name, created_at, updated_at 
		FROM lists 
		WHERE folder_id = $1 
		ORDER BY name
	`
	
	rows, err := r.db.Query(query, folderID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar lists: %w", err)
	}
	defer rows.Close()
	
	var lists []List
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.FolderID, &l.Name, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erro ao escanear list: %w", err)
		}
		lists = append(lists, l)
	}
	
	return lists, nil
}

// GetCustomFields retorna todos os campos personalizados
func (r *MetadataRepository) GetCustomFields() ([]CustomField, error) {
	query := `
		SELECT id, name, type, options, orderindex, created_at, updated_at 
		FROM custom_fields 
		ORDER BY orderindex, name
	`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar custom fields: %w", err)
	}
	defer rows.Close()
	
	var fields []CustomField
	for rows.Next() {
		var f CustomField
		var optionsJSON []byte
		
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &optionsJSON, &f.OrderIndex, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erro ao escanear custom field: %w", err)
		}
		
		// Deserializa options
		if len(optionsJSON) > 0 {
			if err := json.Unmarshal(optionsJSON, &f.Options); err != nil {
				return nil, fmt.Errorf("erro ao deserializar options: %w", err)
			}
		}
		
		fields = append(fields, f)
	}
	
	return fields, nil
}


// GetCustomFieldByID retorna um campo personalizado pelo ID
func (r *MetadataRepository) GetCustomFieldByID(fieldID string) (*CustomField, error) {
	query := `
		SELECT id, name, type, options, orderindex, created_at, updated_at 
		FROM custom_fields 
		WHERE id = $1
	`
	
	var f CustomField
	var optionsJSON []byte
	
	err := r.db.QueryRow(query, fieldID).Scan(&f.ID, &f.Name, &f.Type, &optionsJSON, &f.OrderIndex, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar custom field: %w", err)
	}
	
	// Deserializa options
	if len(optionsJSON) > 0 {
		if err := json.Unmarshal(optionsJSON, &f.Options); err != nil {
			return nil, fmt.Errorf("erro ao deserializar options: %w", err)
		}
	}
	
	return &f, nil
}
