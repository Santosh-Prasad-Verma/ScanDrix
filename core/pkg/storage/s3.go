package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/codehound/codehound/core/pkg/config"
)

// S3Client wraps the AWS S3 client with high-level helpers.
type S3Client struct {
	client     *s3.Client
	presign    *s3.PresignClient
	bucketName string
}

// NewS3Client creates a new S3Client instance targeting LocalStack or real AWS.
func NewS3Client(ctx context.Context, cfg *config.AppConfig) (*S3Client, error) {
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
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	s3Opts := func(o *s3.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
			o.UsePathStyle = true // Required for LocalStack and MinIO
		}
	}

	client := s3.NewFromConfig(awsCfg, s3Opts)
	presignClient := s3.NewPresignClient(client)

	return &S3Client{
		client:     client,
		presign:    presignClient,
		bucketName: cfg.ArtifactsBucket,
	}, nil
}

// PutObject uploads raw bytes to S3 with metadata.
func (s *S3Client) PutObject(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to put object in s3: %w", err)
	}

	return fmt.Sprintf("s3://%s/%s", s.bucketName, key), nil
}

// GetObject downloads an object from S3.
func (s *S3Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from s3: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return data, nil
}

// GeneratePresignedDownloadURL generates a secure temporary download link.
func (s *S3Client) GeneratePresignedDownloadURL(ctx context.Context, key string, lifetime time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", fmt.Errorf("failed to presign get object url: %w", err)
	}

	return req.URL, nil
}

// EnsureBucketExists verifies the target S3 bucket is created.
func (s *S3Client) EnsureBucketExists(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucketName),
	})
	if err == nil {
		return nil
	}

	_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucketName),
	})
	if createErr != nil {
		return fmt.Errorf("failed to create bucket %s: %w", s.bucketName, createErr)
	}

	return nil
}
