package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BaseEntity provides standard primary key and audit timestamps matching CoreModel.
type BaseEntity struct {
	ID        uuid.UUID `json:"id" db:"id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// TenantScopedEntity enforces workspace/tenant isolation for all multi-tenant tables.
type TenantScopedEntity struct {
	BaseEntity
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
}

// JSONBMap represents a generic string-to-interface JSONB document.
type JSONBMap map[string]any

// Value serializes JSONBMap to a database driver value.
func (j JSONBMap) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

// Scan deserializes a database value into JSONBMap.
func (j *JSONBMap) Scan(value any) error {
	if value == nil {
		*j = make(map[string]any)
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal JSONBMap from %T", value)
	}
	return json.Unmarshal(bytes, j)
}

// StringSlice represents a PostgreSQL text array ([]string).
type StringSlice []string

// Value serializes StringSlice for database insertion.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

// Scan deserializes a database value into StringSlice.
func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = make([]string, 0)
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal StringSlice from %T", value)
	}
	return json.Unmarshal(bytes, s)
}

// FloatVector represents a 1536-dimensional or 768-dimensional pgvector embedding.
type FloatVector []float32

// Value formats the float slice as a pgvector literal string "[0.123,0.456,...]".
func (v FloatVector) Value() (driver.Value, error) {
	if len(v) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan parses a pgvector literal string or bytes into a FloatVector.
func (v *FloatVector) Scan(value any) error {
	if value == nil {
		*v = nil
		return nil
	}
	var str string
	switch val := value.(type) {
	case string:
		str = val
	case []byte:
		str = string(val)
	default:
		return fmt.Errorf("cannot scan type %T into FloatVector", value)
	}
	if len(str) < 2 || str[0] != '[' || str[len(str)-1] != ']' {
		return errors.New("invalid pgvector literal format")
	}
	return json.Unmarshal([]byte(str), v)
}
