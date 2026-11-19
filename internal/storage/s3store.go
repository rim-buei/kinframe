package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rim-buei/kinframe/internal/config"
)

// S3Store is the production object store (including Glacier Flexible Retrieval).
type S3Store struct {
	client *s3.Client
	bucket string
}

// NewS3Store builds an S3 client from config (supports custom endpoints / MinIO).
func NewS3Store(ctx context.Context, cfg config.Config) (*S3Store, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET required")
	}
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(cfg.S3Region))
	if cfg.S3AccessKeyID != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "",
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		}
	})
	return &S3Store{client: client, bucket: cfg.S3Bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, in)
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isInvalidObjectState(err) {
			return nil, ErrObjectArchived
		}
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Store) PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	ps := s3.NewPresignClient(s.client)
	in := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := ps.PresignPutObject(ctx, in, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *S3Store) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	ps := s3.NewPresignClient(s.client)
	out, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

// TransitionStorageClass copies the object onto itself with a new storage class
// (used for hot → Glacier Flexible Retrieval and restore completion → Standard).
func (s *S3Store) TransitionStorageClass(ctx context.Context, key string, storageClass string) error {
	class := types.StorageClass(strings.ToUpper(storageClass))
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(s.bucket),
		Key:               aws.String(key),
		CopySource:        aws.String(s.bucket + "/" + key),
		StorageClass:      class,
		MetadataDirective: types.MetadataDirectiveCopy,
	})
	return err
}

// RestoreObject starts a Glacier restore (Flexible Retrieval Standard tier by default).
func (s *S3Store) RestoreObject(ctx context.Context, key string, days int, tier string) error {
	if days <= 0 {
		days = 7
	}
	if tier == "" {
		tier = "Standard"
	}
	_, err := s.client.RestoreObject(ctx, &s3.RestoreObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		RestoreRequest: &types.RestoreRequest{
			Days: aws.Int32(int32(days)),
			GlacierJobParameters: &types.GlacierJobParameters{
				Tier: types.Tier(tier),
			},
		},
	})
	return err
}

func isInvalidObjectState(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalidobjectstate") || strings.Contains(msg, "restore already in progress")
}
