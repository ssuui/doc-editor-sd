package s3uploader

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"doc-publish-server/internal/auth"
	"doc-publish-server/internal/configloader"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Service struct {
	cfg     *configloader.SystemConfig
	client  *s3.Client
	presign *s3.PresignClient
}

func New(cfg *configloader.SystemConfig) (*Service, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.S3.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3.AccessKeyID, cfg.S3.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3.Endpoint != "" {
			o.BaseEndpoint = &cfg.S3.Endpoint
			o.UsePathStyle = true
		}
	})
	return &Service{cfg: cfg, client: client, presign: s3.NewPresignClient(client)}, nil
}

func (s *Service) UploadDir(localDir string, prefix string) (string, error) {
	var logs []string
	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(localDir, path)
		key := strings.TrimLeft(filepath.ToSlash(filepath.Join(prefix, rel)), "/")
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		cacheControl := s.cfg.S3.CacheStatic
		if filepath.Ext(path) == ".html" {
			cacheControl = s.cfg.S3.CacheHTML
		}
		_, err = s.client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:       &s.cfg.S3.DefaultBucketName,
			Key:          &key,
			Body:         bytes.NewReader(body),
			ContentType:  &contentType,
			CacheControl: &cacheControl,
			ACL:          types.ObjectCannedACLPublicRead,
		})
		if err != nil {
			return err
		}
		logs = append(logs, fmt.Sprintf("[UPLOAD] %s", key))
		return nil
	})
	return strings.Join(logs, "\n"), err
}

func (s *Service) PresignImageUpload(bookDir string, ext string) (string, string, error) {
	key := strings.TrimLeft(filepath.ToSlash(filepath.Join(s.cfg.S3.ImgStorePrefix, bookDir, "static-img", fmt.Sprintf("%d_%s%s", time.Now().Unix(), auth.GenerateToken(), ext))), "/")
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	res, err := s.presign.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &s.cfg.S3.DefaultBucketName,
		Key:         &key,
		ContentType: &contentType,
		ACL:         types.ObjectCannedACLPublicRead,
	}, func(options *s3.PresignOptions) {
		options.Expires = time.Duration(s.cfg.S3.PresignPutExpireMin) * time.Minute
	})
	if err != nil {
		return "", "", err
	}
	cdnURL := fmt.Sprintf("https://%s/%s", strings.TrimRight(s.cfg.S3.ImgCDNDomain, "/"), key)
	return res.URL, cdnURL, nil
}
