package secrets

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/codehound/codehound/core/pkg/config"
)

// SecretsClient provides high-level access to AWS Secrets Manager.
type SecretsClient struct {
	client *secretsmanager.Client
}

// NewSecretsClient creates a new SecretsClient.
func NewSecretsClient(ctx context.Context, cfg *config.AppConfig) (*SecretsClient, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.AWSRegion),
	}

	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for secrets manager: %w", err)
	}

	smOpts := func(o *secretsmanager.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	}

	client := secretsmanager.NewFromConfig(awsCfg, smOpts)
	return &SecretsClient{client: client}, nil
}

// GetSecret retrieves a string secret value by secret ID/name.
func (s *SecretsClient) GetSecret(ctx context.Context, secretName string) (string, error) {
	resp, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret '%s': %w", secretName, err)
	}

	if resp.SecretString != nil {
		return *resp.SecretString, nil
	}

	return string(resp.SecretBinary), nil
}

// PutSecret creates or updates a secret in Secrets Manager.
func (s *SecretsClient) PutSecret(ctx context.Context, secretName, secretValue string) error {
	// Try creating the secret first
	_, err := s.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(secretValue),
	})
	if err == nil {
		return nil
	}

	// If already exists, update value
	_, putErr := s.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(secretValue),
	})
	if putErr != nil {
		return fmt.Errorf("failed to create or update secret '%s': %w", secretName, putErr)
	}

	return nil
}
