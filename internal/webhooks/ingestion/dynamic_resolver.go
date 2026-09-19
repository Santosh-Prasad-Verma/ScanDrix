package ingestion

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// DynamicSecretResolver resolves webhook secrets dynamically from PostgreSQL with a fallback to static/env defaults.
type DynamicSecretResolver struct {
	repo        *database.Repository
	fallbackMap map[string]string
}

// NewDynamicSecretResolver creates a resolver that queries the database repository before falling back to static secrets.
func NewDynamicSecretResolver(repo *database.Repository, fallbackMap map[string]string) *DynamicSecretResolver {
	return &DynamicSecretResolver{
		repo:        repo,
		fallbackMap: fallbackMap,
	}
}

// ResolveSecret resolves the secret by querying PostgreSQL for a repository-level or workspace-level webhook secret.
// If none is found or if the database is unreachable, it falls back to the configured provider static secret.
func (r *DynamicSecretResolver) ResolveSecret(provider models.SCMProvider, repoNamespace string) (string, error) {
	if r.repo != nil && repoNamespace != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		secret, err := r.repo.GetWebhookSecretForRepo(ctx, string(provider), repoNamespace)
		if err == nil && secret != "" {
			return secret, nil
		}
	}

	if r.fallbackMap != nil {
		key := string(provider) + ":" + repoNamespace
		if s, ok := r.fallbackMap[key]; ok && s != "" {
			return s, nil
		}
		if s, ok := r.fallbackMap[string(provider)]; ok && s != "" {
			return s, nil
		}
	}

	return "", nil
}
