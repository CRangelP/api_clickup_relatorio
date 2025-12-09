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
