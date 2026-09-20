package scim

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
)

// SCIMService provides in-memory/database backed SCIM 2.0 provisioning.
type SCIMService struct {
	mu          sync.RWMutex
	users       map[string]SCIMUser
	groups      map[string]SCIMGroup
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
		users:  make(map[string]SCIMUser),
		groups: make(map[string]SCIMGroup),
		repo:   r,
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
		if !strings.HasPrefix(authHeader, "Bearer ") {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing Bearer authorization token")
			return
		}
		providedToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
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
		gu.Route("/Groups", func(g chi.Router) {
			g.Get("/", s.handleListGroups)
			g.Post("/", s.handleCreateGroup)
			g.Get("/{id}", s.handleGetGroup)
			g.Put("/{id}", s.handleUpdateGroup)
			g.Patch("/{id}", s.handlePatchGroup)
			g.Delete("/{id}", s.handleDeleteGroup)
		})
	})

	return r
}

func (s *SCIMService) handleServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]bool{"supported": true},
		"bulk":           map[string]bool{"supported": false},
		"filter":         map[string]any{"supported": true, "maxResults": 100},
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

	// Deprovisioning Enforcement (§8.3): If user was deactivated, revoke all active sessions & tokens
	if !user.Active && s.repo != nil {
		email := user.UserName
		if len(user.Emails) > 0 && user.Emails[0].Value != "" {
			email = user.Emails[0].Value
		}
		if dbUser, err := s.repo.GetUserByEmail(r.Context(), email); err == nil && dbUser != nil {
			_ = s.repo.InvalidateAllUserRefreshTokens(r.Context(), dbUser.UUID)
			_ = s.repo.UpdateUserStatus(r.Context(), dbUser.UUID, "suspended")
			slog.Info("SCIM deprovisioning: user sessions revoked and account suspended", "user_id", dbUser.UUID, "email", email)
		}
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *SCIMService) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	s.mu.Lock()
	user, exists := s.users[id]
	if exists {
		delete(s.users, id)
	}
	s.mu.Unlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	// Deprovisioning Enforcement (§8.3): Delete user sessions and revoke all active refresh tokens
	if s.repo != nil {
		email := user.UserName
		if len(user.Emails) > 0 && user.Emails[0].Value != "" {
			email = user.Emails[0].Value
		}
		if dbUser, err := s.repo.GetUserByEmail(r.Context(), email); err == nil && dbUser != nil {
			_ = s.repo.InvalidateAllUserRefreshTokens(r.Context(), dbUser.UUID)
			_ = s.repo.UpdateUserStatus(r.Context(), dbUser.UUID, "inactive")
			slog.Info("SCIM deprovisioning (delete): user sessions revoked and account deactivated", "user_id", dbUser.UUID, "email", email)
		}
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

func (s *SCIMService) handleListGroups(w http.ResponseWriter, r *http.Request) {
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

	allGroups := make([]SCIMGroup, 0, len(s.groups))
	for _, g := range s.groups {
		allGroups = append(allGroups, g)
	}

	totalResults := len(allGroups)
	start := startIndex - 1
	end := start + count
	if start > totalResults {
		start = totalResults
	}
	if end > totalResults {
		end = totalResults
	}

	paged := allGroups[start:end]

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(paged),
		Resources:    paged,
	})
}

func (s *SCIMService) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var group SCIMGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM Group JSON")
		return
	}

	if strings.TrimSpace(group.DisplayName) == "" {
		s.writeError(w, http.StatusBadRequest, "invalidValue", "displayName is required for SCIM Group")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if group.ID == "" {
		group.ID = uuid.New().String()
	}
	group.Schemas = []string{GroupSchemaURN}
	now := time.Now().UTC()
	group.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      now,
		LastModified: now,
		Location:     r.URL.Path + "/" + group.ID,
	}
	if group.Members == nil {
		group.Members = []SCIMGroupMember{}
	}

	s.groups[group.ID] = group

	w.Header().Set("Content-Type", "application/scim+json")
	w.Header().Set("Location", group.Meta.Location)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(group)
}

func (s *SCIMService) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	s.mu.RLock()
	group, exists := s.groups[id]
	s.mu.RUnlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(group)
}

func (s *SCIMService) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var updated SCIMGroup
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM Group JSON")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.groups[id]
	if !exists {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	updated.ID = id
	updated.Schemas = []string{GroupSchemaURN}
	updated.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      existing.Meta.Created,
		LastModified: time.Now().UTC(),
		Location:     existing.Meta.Location,
	}
	if updated.Members == nil {
		updated.Members = []SCIMGroupMember{}
	}

	s.groups[id] = updated

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (s *SCIMService) handlePatchGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var patchReq SCIMPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&patchReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM PATCH payload")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[id]
	if !exists {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	for _, op := range patchReq.Operations {
		opLower := strings.ToLower(op.Op)
		pathLower := strings.ToLower(op.Path)

		switch opLower {
		case "replace":
			if pathLower == "displayname" {
				if strVal, ok := op.Value.(string); ok && strVal != "" {
					group.DisplayName = strVal
				}
			}
		case "add":
			if pathLower == "members" || pathLower == "" {
				if membersList, ok := op.Value.([]any); ok {
					for _, m := range membersList {
						if mMap, ok := m.(map[string]any); ok {
							val, _ := mMap["value"].(string)
							disp, _ := mMap["display"].(string)
							if val != "" {
								group.Members = append(group.Members, SCIMGroupMember{Value: val, Display: disp})
							}
						}
					}
				}
			}
		case "remove":
			if strings.HasPrefix(pathLower, "members") {
				group.Members = []SCIMGroupMember{}
			}
		}
	}

	group.Meta.LastModified = time.Now().UTC()
	s.groups[id] = group

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(group)
}

func (s *SCIMService) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	s.mu.Lock()
	_, exists := s.groups[id]
	if exists {
		delete(s.groups, id)
	}
	s.mu.Unlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

