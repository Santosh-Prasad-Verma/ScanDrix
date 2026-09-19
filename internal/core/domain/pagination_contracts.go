package domain

// PaginationQuery provides standard cursor/offset pagination criteria.
type PaginationQuery struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Cursor   string `json:"cursor,omitempty"`
	OrderBy  string `json:"order_by,omitempty"`
	OrderDir string `json:"order_dir,omitempty"` // "ASC" or "DESC"
}

// EnsureDefaults applies sane defaults to pagination queries.
func (p *PaginationQuery) EnsureDefaults() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 25
	}
	if p.OrderDir == "" {
		p.OrderDir = "DESC"
	}
	if p.OrderBy == "" {
		p.OrderBy = "created_at"
	}
}

// PaginatedResult wraps a slice of items with total counts and pagination metadata.
type PaginatedResult[T any] struct {
	Items      []T    `json:"items"`
	TotalCount int64  `json:"total_count"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	TotalPages int    `json:"total_pages"`
	HasNext    bool   `json:"has_next"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// PaginationDto mirrors ScanDrix PaginationDto.
type PaginationDto struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Skip  int `json:"skip,omitempty"`
}

// Normalize ensures PaginationDto has valid bounds (page >= 1, limit 1-1000, default 100).
func (p *PaginationDto) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	if p.Limit > 1000 {
		p.Limit = 1000
	}
	if p.Skip < 0 {
		p.Skip = 0
	}
}
