package model

// TaskResponse representa a resposta da API do ClickUp para listagem de tarefas
type TaskResponse struct {
	Tasks    []Task `json:"tasks"`
	LastPage bool   `json:"last_page"`
}

// Task representa uma tarefa do ClickUp
type Task struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Status       Status        `json:"status"`
	DateCreated  string        `json:"date_created"`
	DateUpdated  string        `json:"date_updated"`
	DateClosed   string        `json:"date_closed"`
	DueDate      string        `json:"due_date"`
	StartDate    string        `json:"start_date"`
	Priority     *Priority     `json:"priority"`
	Assignees    []Assignee    `json:"assignees"`
	Tags         []Tag         `json:"tags"`
	CustomFields []CustomField `json:"custom_fields"`
	List         ListInfo      `json:"list"`
	Folder       FolderInfo    `json:"folder"`
	Space        SpaceInfo     `json:"space"`
	URL          string        `json:"url"`
}

// Status representa o status de uma tarefa
type Status struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Color      string `json:"color"`
	Type       string `json:"type"`
	Orderindex int    `json:"orderindex"`
}

// Priority representa a prioridade de uma tarefa
type Priority struct {
	ID         string `json:"id"`
	Priority   string `json:"priority"`
	Color      string `json:"color"`
	Orderindex string `json:"orderindex"`
}

// Assignee representa um responsável pela tarefa
type Assignee struct {
	ID             int    `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	Color          string `json:"color"`
	ProfilePicture string `json:"profilePicture"`
}

// Tag representa uma tag da tarefa
type Tag struct {
	Name    string `json:"name"`
	TagFg   string `json:"tag_fg"`
	TagBg   string `json:"tag_bg"`
	Creator int    `json:"creator"`
}

// CustomField representa um campo personalizado
type CustomField struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	TypeConfig   *TypeConfig `json:"type_config"`
	Value        interface{} `json:"value"`
	DateCreated  string      `json:"date_created"`
	HideFromGuests bool      `json:"hide_from_guests"`
	Required     bool        `json:"required"`
}

// TypeConfig contém a configuração específica do tipo de campo
type TypeConfig struct {
	// Para dropdowns
	Default     int      `json:"default"`
	Placeholder string   `json:"placeholder"`
	Options     []Option `json:"options"`

	// Para campos de data
	IncludeTime bool `json:"include_time"`

	// Para campos de moeda
	Precision     int    `json:"precision"`
	CurrencyType  string `json:"currency_type"`

	// Para campos de número
	IsTime bool `json:"is_time"`
}

// Option representa uma opção de dropdown
type Option struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Orderindex int    `json:"orderindex"`
}

// ListInfo informações básicas da lista
type ListInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Access bool   `json:"access"`
}

// FolderInfo informações básicas da pasta
type FolderInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
	Access bool   `json:"access"`
}

// SpaceInfo informações básicas do espaço
type SpaceInfo struct {
	ID string `json:"id"`
}

// Metadata API Response Types

// WorkspaceResponse representa a resposta da API para workspaces
type WorkspaceResponse struct {
	Teams []Workspace `json:"teams"`
}

// Workspace representa um workspace do ClickUp
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SpaceResponse representa a resposta da API para spaces
type SpaceResponse struct {
	Spaces []Space `json:"spaces"`
}

// Space representa um space do ClickUp
type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FolderResponse representa a resposta da API para folders
type FolderResponse struct {
	Folders []Folder `json:"folders"`
}

// Folder representa um folder do ClickUp
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListResponse representa a resposta da API para listas
type ListResponse struct {
	Lists []List `json:"lists"`
}

// List representa uma lista do ClickUp
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CustomFieldResponse representa a resposta da API para campos personalizados
type CustomFieldResponse struct {
	Fields []CustomFieldMetadata `json:"fields"`
}

// CustomFieldMetadata representa metadados de um campo personalizado
type CustomFieldMetadata struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	TypeConfig *TypeConfig           `json:"type_config"`
	Required   bool                   `json:"required"`
}

// UserResponse representa a resposta da API para informações do usuário
type UserResponse struct {
	User UserInfo `json:"user"`
}

// UserInfo representa informações básicas do usuário
type UserInfo struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}
