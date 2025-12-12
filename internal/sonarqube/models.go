package sonarqube

// Metric represents a SonarQube metric
type Metric struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Domain      string `json:"domain"`
	Direction   int    `json:"direction"`
	Qualitative bool   `json:"qualitative"`
	Hidden      bool   `json:"hidden"`
}

// MetricsResponse represents the response from /api/metrics/search
type MetricsResponse struct {
	Metrics []Metric `json:"metrics"`
	Total   int      `json:"total"`
	P       int      `json:"p"`
	PS      int      `json:"ps"`
}

// Component represents a SonarQube project
type Component struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Qualifier    string   `json:"qualifier"`
	IsFavorite   bool     `json:"isFavorite"`
	AnalysisDate string   `json:"analysisDate,omitempty"`
	Tags         []string `json:"tags"`
	Visibility   string   `json:"visibility"`
}

// Paging represents pagination information
type Paging struct {
	PageIndex int `json:"pageIndex"`
	PageSize  int `json:"pageSize"`
	Total     int `json:"total"`
}

// ComponentsResponse represents the response from /api/components/search_projects
type ComponentsResponse struct {
	Paging     Paging      `json:"paging"`
	Components []Component `json:"components"`
}

// MeasuresResponse represents the response from /api/measures/component
type MeasuresResponse struct {
	Component ComponentMeasures `json:"component"`
}

// ComponentMeasures represents a component with its measures
type ComponentMeasures struct {
	Key      string    `json:"key"`
	Name     string    `json:"name"`
	Measures []Measure `json:"measures"`
}

// Measure represents a single measure value
type Measure struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
}
