package model

// TaskEstimate contém a estimativa de tasks por lista
type TaskEstimate struct {
	ListID       string `json:"list_id"`
	ListName     string `json:"list_name"`
	EstimatedMin int    `json:"estimated_min"`
	EstimatedMax int    `json:"estimated_max"`
	IsExact      bool   `json:"is_exact"` // true se <= 100 tasks (contagem exata)
}

// EstimateResult contém o resultado da estimativa total
type EstimateResult struct {
	Lists         []TaskEstimate `json:"lists"`
	TotalMin      int            `json:"total_min"`
	TotalMax      int            `json:"total_max"`
	EstimatedAvg  int            `json:"estimated_avg"`
	EstimatedTime string         `json:"estimated_time"` // Ex: "2-5 minutos"
}
