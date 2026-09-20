package filters

import (
	"fmt"
	"net/http"
)

// APIError is an interface for errors that specify their HTTP status code.
type APIError interface {
	error
	StatusCode() int
}

// CustomDomainException defines structured metadata for domain-level exceptions.
type CustomDomainException interface {
	error
	StatusCode() int
	ErrorKey() string
	Code() string
	Details() interface{}
}

// ConfigurationMissingException indicates an essential configuration parameter is absent.
type ConfigurationMissingException struct {
	ParamName string
}

func (e *ConfigurationMissingException) Error() string {
	return fmt.Sprintf("Configuration parameter missing: %s", e.ParamName)
}
func (e *ConfigurationMissingException) StatusCode() int    { return http.StatusBadRequest }
func (e *ConfigurationMissingException) ErrorKey() string   { return "CONFIGURATION_MISSING" }
func (e *ConfigurationMissingException) Code() string       { return "ERR_CONFIG_MISSING" }
func (e *ConfigurationMissingException) Details() interface{} { return map[string]string{"param": e.ParamName} }

// DuplicateRecordException indicates an entity already exists in the system.
type DuplicateRecordException struct {
	Entity string
	Key    string
}

func (e *DuplicateRecordException) Error() string {
	return fmt.Sprintf("Duplicate %s record with key: %s", e.Entity, e.Key)
}
func (e *DuplicateRecordException) StatusCode() int    { return http.StatusConflict }
func (e *DuplicateRecordException) ErrorKey() string   { return "DUPLICATE_RECORD" }
func (e *DuplicateRecordException) Code() string       { return "ERR_DUPLICATE" }
func (e *DuplicateRecordException) Details() interface{} {
	return map[string]string{"entity": e.Entity, "key": e.Key}
}

// RepositoryWithDirectoriesException indicates review failure due to nested directory constraints.
type RepositoryWithDirectoriesException struct {
	RepoName string
}

func (e *RepositoryWithDirectoriesException) Error() string {
	return fmt.Sprintf("Repository %s requires directory path configuration", e.RepoName)
}
func (e *RepositoryWithDirectoriesException) StatusCode() int    { return http.StatusUnprocessableEntity }
func (e *RepositoryWithDirectoriesException) ErrorKey() string   { return "REPOSITORY_DIRECTORIES_REQUIRED" }
func (e *RepositoryWithDirectoriesException) Code() string       { return "ERR_REPO_DIRECTORIES" }
func (e *RepositoryWithDirectoriesException) Details() interface{} {
	return map[string]string{"repository": e.RepoName}
}
