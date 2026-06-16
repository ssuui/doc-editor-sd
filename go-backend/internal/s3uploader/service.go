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
			// 腾讯云 COS 必须使用 virtual-hosted-style 域名
			// (https://<bucket>.cos.<region>.myqcloud.com)。path-style
			// 会被 COS 拒绝 (PathStyleDomainForbidden),其 403 响应不带
			// CORS 头,浏览器预检时会误报成 CORS 错误。
			o.UsePathStyle = false
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

// isImageExt 判断扩展名是否为图片。图片归入 static-img 目录,
// 其它附件(pdf/zip/docx 等)归入 static-asset 目录,避免混放。
func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return true
	}
	return false
}

// PresignImageUpload 为上传生成预签名 PUT URL。返回的 contentType 与 acl
// 必须由客户端在 PUT 请求头中原样回传(Content-Type / x-amz-acl),因为它们
// 是 SigV4 签名的一部分 —— 不一致会返回 403 SignatureDoesNotMatch。
//
// 存储目录按文件类型分流:图片 -> static-img,其它附件 -> static-asset。
func (s *Service) PresignImageUpload(bookDir string, ext string) (putURL, cdnURL, contentType, acl string, err error) {
	storeDir := "static-asset"
	if isImageExt(ext) {
		storeDir = "static-img"
	}
	key := strings.TrimLeft(filepath.ToSlash(filepath.Join(s.cfg.S3.ImgStorePrefix, bookDir, storeDir, fmt.Sprintf("%d_%s%s", time.Now().Unix(), auth.GenerateToken(), ext))), "/")
	contentType = mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	acl = string(types.ObjectCannedACLPublicRead)
	res, perr := s.presign.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &s.cfg.S3.DefaultBucketName,
		Key:         &key,
		ContentType: &contentType,
		ACL:         types.ObjectCannedACLPublicRead,
	}, func(options *s3.PresignOptions) {
		options.Expires = time.Duration(s.cfg.S3.PresignPutExpireMin) * time.Minute
	})
	if perr != nil {
		return "", "", "", "", perr
	}
	cdnURL = fmt.Sprintf("https://%s/%s", strings.TrimRight(s.cfg.S3.ImgCDNDomain, "/"), key)
	return res.URL, cdnURL, contentType, acl, nil
}
