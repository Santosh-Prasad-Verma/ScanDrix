package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// ITeamCliKeyService defines contract for validating team API keys.
type ITeamCliKeyService = usecases.ITeamCliKeyService

// TeamCliKeyData holds team and organization entities linked to an API key.
type TeamCliKeyData = usecases.TeamCliKeyData

// TeamEntityRef represents an organizational team.
type TeamEntityRef = usecases.TeamEntityRef

// OrganizationEntityRef represents an enterprise organization.
type OrganizationEntityRef = usecases.OrganizationEntityRef

// ITeamService provides team lookup capabilities.
type ITeamService = usecases.ITeamService

// IAuthService validates active user accounts.
type IAuthService = usecases.IAuthService

// AuthUserRecord represents an authenticated system user.
type AuthUserRecord = usecases.AuthUserRecord

// ICliDeviceService manages terminal device validation and token registration.
type ICliDeviceService = usecases.ICliDeviceService

// DeviceRegistrationInput parameterizes device checks.
type DeviceRegistrationInput = usecases.DeviceRegistrationInput

// DeviceRegistrationResult contains minted or validated device token.
type DeviceRegistrationResult = usecases.DeviceRegistrationResult

// ValidateCliKeyUseCase encapsulates team API key and JWT authentication with device tracking.
type ValidateCliKeyUseCase = usecases.ValidateCliKeyUseCase

// NewValidateCliKeyUseCase instantiates the key validation use case.
var NewValidateCliKeyUseCase = usecases.NewValidateCliKeyUseCase
