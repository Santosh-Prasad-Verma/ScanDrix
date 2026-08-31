package scim

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
)

// SCIMService provides in-memory/database backed SCIM 2.0 provisioning.
type SCIMService struct {
	mu          sync.RWMutex
	users       map[string]SCIMUser
	repo        *database.Repository
	bearerToken string
}

// NewSCIMService initializes the SCIM 2.0 provisioning engine with optional database persistence.
func NewSCIMService(repo ...*database.Repository) *SCIMService {
	var r *database.Repository
	if len(repo) > 0 {
		r = repo[0]
	}
	return &SCIMService{
		users: make(map[string]SCIMUser),
		repo:  r,
	}
}

// SetBearerToken configures the required Bearer token for SCIM operations.
func (s *SCIMService) SetBearerToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bearerToken = token
}

func (s *SCIMService) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		token := s.bearerToken
		s.mu.RUnlock()

		if token == "" {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "SCIM 2.0 provisioning token is not configured on the server")
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) != token {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing Bearer authorization token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Routes mounts the standard SCIM 2.0 sub-router.
func (s *SCIMService) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/ServiceProviderConfig", s.handleServiceProviderConfig)
	r.Get("/Schemas", s.handleSchemas)

	r.Group(func(gu chi.Router) {
		gu.Use(s.authMiddleware)
		gu.Route("/Users", func(u chi.Router) {
			u.Get("/", s.handleListUsers)
			u.Post("/", s.handleCreateUser)
			u.Get("/{id}", s.handleGetUser)
			u.Patch("/{id}", s.handlePatchUser)
			u.Delete("/{id}", s.handleDeleteUser)
		})
	})

	return r
}

func (s *SCIMService) handleServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":   map[string]bool{"supported": true},
		"bulk":    map[string]bool{"supported": false},
		"filter":  map[string]any{"supported": true, "maxResults": 100},
		"changePassword": map[string]bool{"supported": false},
		"sort":           map[string]bool{"supported": false},
		"etag":           map[string]bool{"supported": false},
		"authenticationSchemes": []map[string]any{
			{
				"name":        "OAuth Bearer Token",
				"description": "Authentication scheme using the OAuth Bearer Token Standard",
				"specUri":     "http://www.rfc-editor.org/info/rfc6750",
				"type":        "oauthbearertoken",
				"primary":     true,
			},
		},
	})
}

func (s *SCIMService) handleSchemas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: 2,
		StartIndex:   1,
		ItemsPerPage: 2,
		Resources: []map[string]any{
			{"id": UserSchemaURN, "name": "User"},
			{"id": GroupSchemaURN, "name": "Group"},
		},
	})
}

func (s *SCIMService) handleListUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startIndex, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
	if startIndex < 1 {
		startIndex = 1
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 || count > 100 {
		count = 100
	}

	filter := r.URL.Query().Get("filter")

	userList := make([]SCIMUser, 0, len(s.users))
	for _, u := range s.users {
		if filter != "" {
			// Basic filter support: userName eq "..."
			if strings.Contains(filter, "userName eq") {
				parts := strings.Split(filter, "\"")
				if len(parts) >= 2 && u.UserName != parts[1] {
					continue
				}
			}
		}
		userList = append(userList, u)
	}

	if len(userList) > count {
		userList = userList[:count]
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: len(userList),
		StartIndex:   startIndex,
		ItemsPerPage: len(userList),
		Resources:    userList,
	})
}

func (s *SCIMService) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var input SCIMUser
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Invalid SCIM JSON payload")
		return
	}

	if input.UserName == "" {
		s.writeError(w, http.StatusBadRequest, "invalidValue", "userName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newID := uuid.New().String()
	input.ID = newID
	input.Schemas = []string{UserSchemaURN}
	input.Meta.ResourceType = "User"
	input.Meta.Location = "/scim/v2/Users/" + newID

	s.users[newID] = input

	if s.repo != nil {
		email := input.UserName
		if len(input.Emails) > 0 && input.Emails[0].Value != "" {
			email = input.Emails[0].Value
		}
		_, _ = s.repo.CreateUser(r.Context(), email, "scim-sso-managed", "member", nil)
	}

	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(input)
}

func (s *SCIMService) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.mu.RLock()
	user, exists := s.users[id]
	s.mu.RUnlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *SCIMService) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var patch SCIMPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Invalid SCIM patch payload")
		return
	}

	s.mu.Lock()
	user, exists := s.users[id]
	if !exists {
		s.mu.Unlock()
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	for _, op := range patch.Operations {
		if strings.EqualFold(op.Op, "replace") {
			if strings.EqualFold(op.Path, "active") {
				if activeBool, ok := op.Value.(bool); ok {
					user.Active = activeBool
				}
			}
		}
	}
	s.users[id] = user
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *SCIMService) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	s.mu.Lock()
	_, exists := s.users[id]
	if exists {
		delete(s.users, id)
	}
	s.mu.Unlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *SCIMService) writeError(w http.ResponseWriter, statusCode int, scimType, detail string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(SCIMError{
		Schemas:  []string{ErrorSchemaURN},
		Status:   strconv.Itoa(statusCode),
		ScimType: scimType,
		Detail:   detail,
	})
}
