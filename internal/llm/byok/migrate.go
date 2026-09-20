// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

import (
	"encoding/json"
	"reflect"
	"strings"
)

// LegacySlot represents the legacy stored BYOK slot format.
type LegacySlot struct {
	Provider                string   `json:"provider,omitempty"`
	APIKey                  string   `json:"apiKey,omitempty"`
	Model                   string   `json:"model,omitempty"`
	BaseURL                 string   `json:"baseURL,omitempty"`
	ReasoningEffort         string   `json:"reasoningEffort,omitempty"`
	ReasoningConfigOverride string   `json:"reasoningConfigOverride,omitempty"`
	Temperature             *float64 `json:"temperature,omitempty"`
	MaxInputTokens          int      `json:"maxInputTokens,omitempty"`
	MaxOutputTokens         int      `json:"maxOutputTokens,omitempty"`
	MaxConcurrentRequests   int      `json:"maxConcurrentRequests,omitempty"`
	VertexLocation          string   `json:"vertexLocation,omitempty"`
	AWSRegion               string   `json:"awsRegion,omitempty"`
	OpenRouterProviderOrder []string `json:"openrouterProviderOrder,omitempty"`
	OpenRouterAllowFallback *bool    `json:"openrouterAllowFallbacks,omitempty"`
	AWSBearerToken          string   `json:"awsBearerToken,omitempty"`
	AWSAccessKeyID          string   `json:"awsAccessKeyId,omitempty"`
	AWSSecretAccessKey      string   `json:"awsSecretAccessKey,omitempty"`
	AWSSessionToken         string   `json:"awsSessionToken,omitempty"`
}

// LegacyConfig represents the stored legacy {main, fallback} BYOK JSON blob.
type LegacyConfig struct {
	Main     *LegacySlot `json:"main,omitempty"`
	Fallback *LegacySlot `json:"fallback,omitempty"`
}

func strClean(v string) string {
	return strings.TrimSpace(v)
}

func hasAuth(slot *LegacySlot) bool {
	if slot == nil {
		return false
	}
	return strClean(slot.APIKey) != "" ||
		strClean(slot.AWSBearerToken) != "" ||
		(strClean(slot.AWSAccessKeyID) != "" && strClean(slot.AWSSecretAccessKey) != "")
}

func isUsableSlot(slot *LegacySlot) bool {
	if slot == nil {
		return false
	}
	return strClean(slot.Provider) != "" &&
		strClean(slot.Model) != "" &&
		hasAuth(slot)
}

// plaintextEquals compares the decrypted values of two ciphertexts in local scope.
// If either decryption fails (e.g. key rotation or corruption), it degrades safely
// to false ("treat as distinct credentials") without aborting the migration.
func plaintextEquals(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	decA, errA := DecryptKey(a)
	decB, errB := DecryptKey(b)
	if errA != nil || errB != nil {
		return false
	}
	return decA == decB
}

// sameCredential determines whether two legacy slots resolve to the identical credential.
// Requires matching provider, plaintext apiKey, connection settings (baseURL, vertexLocation, awsRegion),
// secret settings (awsBearerToken, awsAccessKeyId, awsSecretAccessKey, awsSessionToken),
// and OpenRouter routing preferences.
func sameCredential(a, b *LegacySlot) bool {
	if a == nil || b == nil {
		return false
	}
	if strClean(a.Provider) != strClean(b.Provider) {
		return false
	}
	if !plaintextEquals(a.APIKey, b.APIKey) {
		return false
	}
	if strClean(a.BaseURL) != strClean(b.BaseURL) ||
		strClean(a.VertexLocation) != strClean(b.VertexLocation) ||
		strClean(a.AWSRegion) != strClean(b.AWSRegion) {
		return false
	}

	secretPairs := [][2]string{
		{a.AWSBearerToken, b.AWSBearerToken},
		{a.AWSAccessKeyID, b.AWSAccessKeyID},
		{a.AWSSecretAccessKey, b.AWSSecretAccessKey},
		{a.AWSSessionToken, b.AWSSessionToken},
	}
	for _, pair := range secretPairs {
		av := strClean(pair[0])
		bv := strClean(pair[1])
		if av == "" && bv == "" {
			continue
		}
		if av == "" || bv == "" || !plaintextEquals(av, bv) {
			return false
		}
	}

	if !reflect.DeepEqual(a.OpenRouterProviderOrder, b.OpenRouterProviderOrder) {
		return false
	}
	if (a.OpenRouterAllowFallback == nil) != (b.OpenRouterAllowFallback == nil) {
		return false
	}
	if a.OpenRouterAllowFallback != nil && b.OpenRouterAllowFallback != nil {
		if *a.OpenRouterAllowFallback != *b.OpenRouterAllowFallback {
			return false
		}
	}

	return true
}

func credentialFromSlot(id string, slot *LegacySlot) BYOKCredential {
	settings := make(map[string]any)

	putStr := func(k, v string) {
		if s := strClean(v); s != "" {
			settings[k] = s
		}
	}

	putStr("baseURL", slot.BaseURL)
	putStr("vertexLocation", slot.VertexLocation)
	putStr("awsRegion", slot.AWSRegion)
	putStr("awsBearerToken", slot.AWSBearerToken)
	putStr("awsAccessKeyId", slot.AWSAccessKeyID)
	putStr("awsSecretAccessKey", slot.AWSSecretAccessKey)
	putStr("awsSessionToken", slot.AWSSessionToken)

	if len(slot.OpenRouterProviderOrder) > 0 {
		var orders []string
		for _, o := range slot.OpenRouterProviderOrder {
			if s := strClean(o); s != "" {
				orders = append(orders, s)
			}
		}
		if len(orders) > 0 {
			settings["openrouterProviderOrder"] = orders
		}
	}
	if slot.OpenRouterAllowFallback != nil {
		settings["openrouterAllowFallbacks"] = *slot.OpenRouterAllowFallback
	}

	cred := BYOKCredential{
		ID:       id,
		Provider: strClean(slot.Provider),
		APIKey:   strClean(slot.APIKey),
	}
	if len(settings) > 0 {
		cred.Settings = settings
	}
	return cred
}

func modelFromSlot(id, credID string, slot *LegacySlot) BYOKModelConfig {
	model := BYOKModelConfig{
		ID:                      id,
		CredentialID:            credID,
		Model:                   strClean(slot.Model),
		ReasoningEffort:         slot.ReasoningEffort,
		ReasoningConfigOverride: slot.ReasoningConfigOverride,
		Temperature:             slot.Temperature,
		MaxInputTokens:          slot.MaxInputTokens,
		MaxOutputTokens:         slot.MaxOutputTokens,
		MaxConcurrentRequests:   slot.MaxConcurrentRequests,
	}
	return model
}

// MigrateLegacyToV2 converts a stored legacy {main, fallback} BYOK JSON blob into the v2 BYOKConfig format.
// Invariants:
// - Encrypted key material (apiKey, aws* secrets) is carried ciphertext-verbatim.
// - Plaintext equality comparison runs in memory only for credential deduplication.
// - Already-v2 blobs (Version == 2) are returned unchanged.
// - If neither main nor fallback is usable, returns an empty v2 config (managed/env default).
// - Fallback is promoted to primary when main is absent or non-usable.
func MigrateLegacyToV2(rawJSON []byte) (*BYOKConfig, error) {
	if len(rawJSON) == 0 {
		return &BYOKConfig{Version: 2, Credentials: []BYOKCredential{}, Models: []BYOKModelConfig{}}, nil
	}

	// Check if already v2
	var checkV2 struct {
		Version     int               `json:"version"`
		Credentials []BYOKCredential  `json:"credentials"`
		Models      []BYOKModelConfig `json:"models"`
		Routing     BYOKRouting       `json:"routing"`
	}
	if err := json.Unmarshal(rawJSON, &checkV2); err == nil && checkV2.Version == 2 {
		return &BYOKConfig{
			Version:     checkV2.Version,
			Credentials: checkV2.Credentials,
			Models:      checkV2.Models,
			Routing:     checkV2.Routing,
		}, nil
	}

	var legacy LegacyConfig
	if err := json.Unmarshal(rawJSON, &legacy); err != nil {
		return &BYOKConfig{Version: 2, Credentials: []BYOKCredential{}, Models: []BYOKModelConfig{}}, nil
	}

	mainUsable := isUsableSlot(legacy.Main)
	fallbackUsable := isUsableSlot(legacy.Fallback)

	if !mainUsable && !fallbackUsable {
		return &BYOKConfig{Version: 2, Credentials: []BYOKCredential{}, Models: []BYOKModelConfig{}}, nil
	}

	var primary *LegacySlot
	var secondary *LegacySlot

	if mainUsable {
		primary = legacy.Main
		if fallbackUsable {
			secondary = legacy.Fallback
		}
	} else {
		// Promoted fallback
		primary = legacy.Fallback
	}

	mainCredID := "cred-main"
	mainModelID := "model-main"

	credentials := []BYOKCredential{credentialFromSlot(mainCredID, primary)}
	models := []BYOKModelConfig{modelFromSlot(mainModelID, mainCredID, primary)}

	if secondary != nil {
		same := sameCredential(primary, secondary)
		fallbackCredID := mainCredID
		if !same {
			fallbackCredID = "cred-fallback"
			credentials = append(credentials, credentialFromSlot(fallbackCredID, secondary))
		}
		models = append(models, modelFromSlot("model-fallback", fallbackCredID, secondary))
	}

	return &BYOKConfig{
		Version:     2,
		Credentials: credentials,
		Models:      models,
		Routing: BYOKRouting{
			DefaultModelID: mainModelID,
		},
	}, nil
}
