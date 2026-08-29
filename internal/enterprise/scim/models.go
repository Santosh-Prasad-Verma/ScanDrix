package scim

import (
	"time"

	"github.com/google/uuid"
)

// SCIM Schemas URNs
const (
	UserSchemaURN  = "urn:ietf:params:scim:schemas:core:2.0:User"
	GroupSchemaURN = "urn:ietf:params:scim:schemas:core:2.0:Group"
	ListResponseURN = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	ErrorSchemaURN  = "urn:ietf:params:scim:api:messages:2.0:Error"
)

// SCIMUser represents a SCIM 2.0 User resource.
type SCIMUser struct {
	Schemas    []string       `json:"schemas"`
	ID         string         `json:"id"`
	UserName   string         `json:"userName"`
	Name       SCIMName       `json:"name"`
	Emails     []SCIMEmail    `json:"emails"`
	Active     bool           `json:"active"`
	Meta       SCIMMeta       `json:"meta"`
	Groups     []SCIMGroupRef `json:"groups,omitempty"`
}

// SCIMName models structured person name components.
type SCIMName struct {
	Formatted  string `json:"formatted"`
	FamilyName string `json:"familyName"`
	GivenName  string `json:"givenName"`
}

// SCIMEmail models a multi-valued email address.
type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type"`
	Primary bool   `json:"primary"`
}

// SCIMGroupRef models a group membership reference.
type SCIMGroupRef struct {
	Value   string `json:"value"`
	Display string `json:"display"`
}

// SCIMGroup represents a SCIM 2.0 Group resource.
type SCIMGroup struct {
	Schemas     []string          `json:"schemas"`
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName"`
	Members     []SCIMGroupMember `json:"members"`
	Meta        SCIMMeta          `json:"meta"`
}

// SCIMGroupMember models a member within a group.
type SCIMGroupMember struct {
	Value   string `json:"value"`
	Display string `json:"display"`
}

// SCIMMeta holds resource metadata.
type SCIMMeta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
}

// SCIMListResponse models standard SCIM 2.0 paginated list payload.
type SCIMListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    any      `json:"Resources"`
}

// SCIMPatchRequest models a SCIM 2.0 PATCH update operation.
type SCIMPatchRequest struct {
	Schemas    []string        `json:"schemas"`
	Operations []SCIMOperation `json:"Operations"`
}

// SCIMOperation represents an individual patch verb (add, remove, replace).
type SCIMOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path,omitempty"`
	Value any    `json:"value"`
}

// SCIMError models a standard SCIM error response.
type SCIMError struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail"`
}

// NewSCIMUser creates a standard SCIM 2.0 user structure from basic credentials.
func NewSCIMUser(id uuid.UUID, email, firstName, lastName string, active bool) SCIMUser {
	now := time.Now().UTC()
	return SCIMUser{
		Schemas:  []string{UserSchemaURN},
		ID:       id.String(),
		UserName: email,
		Name: SCIMName{
			Formatted:  firstName + " " + lastName,
			FamilyName: lastName,
			GivenName:  firstName,
		},
		Emails: []SCIMEmail{
			{Value: email, Type: "work", Primary: true},
		},
		Active: active,
		Meta: SCIMMeta{
			ResourceType: "User",
			Created:      now,
			LastModified: now,
			Location:     "/scim/v2/Users/" + id.String(),
		},
	}
}
