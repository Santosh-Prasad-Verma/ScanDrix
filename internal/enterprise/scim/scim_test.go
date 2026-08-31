package scim_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/scim"
)

func TestSCIMUserLifecycle(t *testing.T) {
	service := scim.NewSCIMService()
	service.SetBearerToken("test-bearer-token")
	router := service.Routes()
	authHdr := "Bearer test-bearer-token"

	// 1. Create User
	newUser := scim.NewSCIMUser(uuid.New(), "alex.turner@arctic.com", "Alex", "Turner", true)
	body, _ := json.Marshal(newUser)

	req := httptest.NewRequest(http.MethodPost, "/Users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/scim+json")
	req.Header.Set("Authorization", authHdr)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var createdUser scim.SCIMUser
	_ = json.NewDecoder(w.Body).Decode(&createdUser)
	if createdUser.UserName != "alex.turner@arctic.com" || createdUser.ID == "" {
		t.Fatalf("unexpected user response: %+v", createdUser)
	}

	// 2. Get User
	reqGet := httptest.NewRequest(http.MethodGet, "/Users/"+createdUser.ID, nil)
	reqGet.Header.Set("Authorization", authHdr)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wGet.Code)
	}

	// 3. Patch User (Deactivate)
	patchReq := scim.SCIMPatchRequest{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []scim.SCIMOperation{
			{Op: "replace", Path: "active", Value: false},
		},
	}
	patchBody, _ := json.Marshal(patchReq)
	reqPatch := httptest.NewRequest(http.MethodPatch, "/Users/"+createdUser.ID, bytes.NewReader(patchBody))
	reqPatch.Header.Set("Content-Type", "application/scim+json")
	reqPatch.Header.Set("Authorization", authHdr)
	wPatch := httptest.NewRecorder()
	router.ServeHTTP(wPatch, reqPatch)

	if wPatch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for patch, got %d", wPatch.Code)
	}
	var patchedUser scim.SCIMUser
	_ = json.NewDecoder(wPatch.Body).Decode(&patchedUser)
	if patchedUser.Active {
		t.Error("expected user active status to be false after patch")
	}

	// 4. Delete User
	reqDel := httptest.NewRequest(http.MethodDelete, "/Users/"+createdUser.ID, nil)
	reqDel.Header.Set("Authorization", authHdr)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", wDel.Code)
	}

	// 5. Verify 404 after deletion
	reqGetAfter := httptest.NewRequest(http.MethodGet, "/Users/"+createdUser.ID, nil)
	reqGetAfter.Header.Set("Authorization", authHdr)
	wGetAfter := httptest.NewRecorder()
	router.ServeHTTP(wGetAfter, reqGetAfter)
	if wGetAfter.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", wGetAfter.Code)
	}
}

func TestSCIMAuthentication(t *testing.T) {
	service := scim.NewSCIMService()
	service.SetBearerToken("secret-scim-token-123")
	router := service.Routes()

	newUser := scim.NewSCIMUser(uuid.New(), "secure.user@arctic.com", "Secure", "User", true)
	body, _ := json.Marshal(newUser)

	// 1. Request without auth header should fail 401
	req := httptest.NewRequest(http.MethodPost, "/Users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/scim+json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Request with invalid token should fail 401
	reqInvalid := httptest.NewRequest(http.MethodPost, "/Users", bytes.NewReader(body))
	reqInvalid.Header.Set("Content-Type", "application/scim+json")
	reqInvalid.Header.Set("Authorization", "Bearer wrong-token")
	wInvalid := httptest.NewRecorder()
	router.ServeHTTP(wInvalid, reqInvalid)

	if wInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad token, got %d", wInvalid.Code)
	}

	// 3. Request with valid token should succeed 201
	reqValid := httptest.NewRequest(http.MethodPost, "/Users", bytes.NewReader(body))
	reqValid.Header.Set("Content-Type", "application/scim+json")
	reqValid.Header.Set("Authorization", "Bearer secret-scim-token-123")
	wValid := httptest.NewRecorder()
	router.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with valid token, got %d: %s", wValid.Code, wValid.Body.String())
	}
}
