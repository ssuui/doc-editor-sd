package publisher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"doc-publish-server/internal/configloader"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type PublishOptions struct {
	Mode       string
	Scope      string
	TargetPath string
	Logf       func(string)
}

type PublishResult struct {
	PublicURL       string
	BackupPath      string
	BackupCreatedAt string
	Files           []configloader.PublishedFileRecord
}

type Publisher interface {
	Validate() error
	TestConnection() error
	PublishDir(localDir string, options PublishOptions) (*PublishResult, error)
	RestoreBackup(record *configloader.PublishRecord) error
}

func New(cfg *configloader.PublishTargetConfig) (Publisher, error) {
	switch cfg.Type {
	case "s3":
		return newS3Publisher(cfg)
	case "local_dir":
		return &localPublisher{cfg: cfg}, nil
	case "sftp":
		return newSFTPPublisher(cfg)
	default:
		return nil, fmt.Errorf("未知发布目标类型: %s", cfg.Type)
	}
}

func collectLocalFiles(localDir string) ([]localFile, error) {
	var files []localFile
	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		files = append(files, localFile{
			AbsPath:  path,
			RelPath:  filepath.ToSlash(rel),
			Size:     info.Size(),
			Checksum: "sha256:" + hex.EncodeToString(sum[:]),
			Data:     raw,
		})
		return nil
	})
	return files, err
}

func emitProgress(options PublishOptions, format string, args ...any) {
	if options.Logf == nil {
		return
	}
	options.Logf(fmt.Sprintf(format, args...))
}

type localFile struct {
	AbsPath  string
	RelPath  string
	Size     int64
	Checksum string
	Data     []byte
}

func topLevelEntries(files []localFile) []string {
	seen := map[string]struct{}{}
	var entries []string
	for _, file := range files {
		name := strings.Split(file.RelPath, "/")[0]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entries = append(entries, name)
	}
	sort.Strings(entries)
	return entries
}

type localPublisher struct {
	cfg *configloader.PublishTargetConfig
}

func (p *localPublisher) Validate() error {
	if p.cfg.TargetDir == "" {
		return fmt.Errorf("target_dir 不能为空")
	}
	return nil
}

func (p *localPublisher) TestConnection() error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(p.cfg.TargetDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(p.cfg.BakDir, 0o755)
}

func (p *localPublisher) PublishDir(localDir string, options PublishOptions) (*PublishResult, error) {
	emitProgress(options, "[PUBLISH] 扫描待发布目录: %s", localDir)
	files, err := collectLocalFiles(localDir)
	if err != nil {
		return nil, err
	}
	emitProgress(options, "[PUBLISH] 已收集 %d 个文件，准备发布到本地目录", len(files))
	destRoot := filepath.Join(p.cfg.TargetDir, filepath.FromSlash(options.TargetPath))
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return nil, fmt.Errorf("创建发布目录失败 %s: %w", destRoot, err)
	}
	result := &PublishResult{}
	if options.Mode == "overwrite" {
		emitProgress(options, "[PUBLISH] 覆盖模式开启，准备备份现有目录")
		version := time.Now().Format("20060102_150405")
		result.BackupCreatedAt = time.Now().Format("2006-01-02 15:04:05")
		result.BackupPath = filepath.ToSlash(filepath.Join(p.cfg.BakDir, version, filepath.FromSlash(options.TargetPath)))
		for _, entry := range topLevelEntries(files) {
			sourcePath := filepath.Join(destRoot, filepath.FromSlash(entry))
			if _, err := os.Stat(sourcePath); err != nil {
				continue
			}
			targetPath := filepath.Join(p.cfg.BakDir, version, filepath.FromSlash(options.TargetPath), filepath.FromSlash(entry))
			emitProgress(options, "[PUBLISH] 备份目录项: %s -> %s", sourcePath, targetPath)
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return nil, fmt.Errorf("创建备份目录失败 %s: %w", filepath.Dir(targetPath), err)
			}
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return nil, fmt.Errorf("移动旧文件到备份目录失败 %s -> %s: %w", sourcePath, targetPath, err)
			}
		}
	}
	for index, file := range files {
		targetPath := filepath.Join(destRoot, filepath.FromSlash(file.RelPath))
		emitProgress(options, "[PUBLISH] 写入文件 %d/%d: %s", index+1, len(files), file.RelPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return nil, fmt.Errorf("创建目标子目录失败 %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, file.Data, 0o644); err != nil {
			return nil, fmt.Errorf("写入发布文件失败 %s: %w", targetPath, err)
		}
		result.Files = append(result.Files, configloader.PublishedFileRecord{
			SourceRelPath: file.RelPath,
			TargetPath:    filepath.ToSlash(targetPath),
			FileSize:      file.Size,
			Checksum:      file.Checksum,
		})
	}
	emitProgress(options, "[PUBLISH] 本地目录发布完成，共写入 %d 个文件", len(files))
	result.PublicURL = filepath.ToSlash(destRoot)
	return result, nil
}

func (p *localPublisher) RestoreBackup(record *configloader.PublishRecord) error {
	if record.BackupPath == "" {
		return fmt.Errorf("当前记录没有备份目录")
	}
	scope := scopePath(record)
	targetRoot := filepath.Join(p.cfg.TargetDir, filepath.FromSlash(scope))
	backupRoot := filepath.FromSlash(record.BackupPath)
	return restoreFromLocalBackup(targetRoot, backupRoot)
}

func restoreFromLocalBackup(targetRoot string, backupRoot string) error {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		current := filepath.Join(targetRoot, entry.Name())
		_ = os.RemoveAll(current)
		if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(backupRoot, entry.Name()), current); err != nil {
			return err
		}
	}
	return nil
}

type s3Publisher struct {
	cfg    *configloader.PublishTargetConfig
	client *s3.Client
}

func newS3Publisher(cfg *configloader.PublishTargetConfig) (Publisher, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = &cfg.Endpoint
			o.UsePathStyle = false
		}
	})
	return &s3Publisher{cfg: cfg, client: client}, nil
}

func (p *s3Publisher) Validate() error {
	if p.cfg.Bucket == "" {
		return fmt.Errorf("bucket 不能为空")
	}
	return nil
}

func (p *s3Publisher) TestConnection() error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := p.client.HeadBucket(context.Background(), &s3.HeadBucketInput{Bucket: &p.cfg.Bucket})
	return err
}

func (p *s3Publisher) PublishDir(localDir string, options PublishOptions) (*PublishResult, error) {
	emitProgress(options, "[PUBLISH] 扫描待发布目录: %s", localDir)
	files, err := collectLocalFiles(localDir)
	if err != nil {
		return nil, err
	}
	emitProgress(options, "[PUBLISH] 已收集 %d 个文件，准备上传到 S3 Bucket %s", len(files), p.cfg.Bucket)
	result := &PublishResult{}
	targetPrefix := joinKey(p.cfg.BasePrefix, options.TargetPath)
	if options.Mode == "overwrite" {
		emitProgress(options, "[PUBLISH] 覆盖模式开启，准备备份 S3 现有对象")
		version := time.Now().Format("20060102_150405")
		result.BackupCreatedAt = time.Now().Format("2006-01-02 15:04:05")
		result.BackupPath = joinKey(p.cfg.BasePrefix, "bak", version, options.TargetPath)
		for _, entry := range topLevelEntries(files) {
			entryPrefix := joinKey(targetPrefix, entry)
			emitProgress(options, "[PUBLISH] 备份 S3 路径: %s -> %s", entryPrefix, joinKey(result.BackupPath, entry))
			if err := p.backupEntry(entryPrefix, joinKey(result.BackupPath, entry)); err != nil {
				return nil, err
			}
		}
	}
	for index, file := range files {
		key := joinKey(targetPrefix, file.RelPath)
		emitProgress(options, "[PUBLISH] 上传文件 %d/%d: %s -> s3://%s/%s", index+1, len(files), file.RelPath, p.cfg.Bucket, key)
		contentType := mime.TypeByExtension(filepath.Ext(file.RelPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		cacheControl := p.cfg.CacheStatic
		if filepath.Ext(file.RelPath) == ".html" {
			cacheControl = p.cfg.CacheHTML
		}
		_, err := p.client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:       &p.cfg.Bucket,
			Key:          &key,
			Body:         bytes.NewReader(file.Data),
			ContentType:  &contentType,
			CacheControl: &cacheControl,
			ACL:          types.ObjectCannedACLPublicRead,
		})
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, configloader.PublishedFileRecord{
			SourceRelPath: file.RelPath,
			TargetPath:    "s3://" + p.cfg.Bucket + "/" + key,
			TargetKey:     key,
			FileSize:      file.Size,
			Checksum:      file.Checksum,
		})
	}
	emitProgress(options, "[PUBLISH] S3 发布完成，共上传 %d 个文件", len(files))
	result.PublicURL = strings.TrimRight(p.cfg.SitePublicDomain, "/")
	if result.PublicURL != "" && !strings.HasPrefix(result.PublicURL, "http") {
		result.PublicURL = "https://" + result.PublicURL
	}
	if options.TargetPath != "" && result.PublicURL != "" {
		result.PublicURL = strings.TrimRight(result.PublicURL, "/") + "/" + strings.TrimLeft(options.TargetPath, "/") + "/"
	}
	return result, nil
}

func (p *s3Publisher) RestoreBackup(record *configloader.PublishRecord) error {
	if record.BackupPath == "" {
		return fmt.Errorf("当前记录没有备份路径")
	}
	scope := scopePath(record)
	sourcePrefix := strings.Trim(record.BackupPath, "/")
	targetPrefix := joinKey(p.cfg.BasePrefix, scope)
	return p.restorePrefix(sourcePrefix, targetPrefix)
}

func (p *s3Publisher) backupEntry(sourcePrefix string, backupPrefix string) error {
	keys, err := p.listKeys(sourcePrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		backupKey := backupPrefix
		if key != strings.Trim(sourcePrefix, "/") {
			backupKey = joinKey(backupPrefix, strings.TrimPrefix(key, strings.TrimRight(strings.Trim(sourcePrefix, "/"), "/")+"/"))
		}
		copySource := p.cfg.Bucket + "/" + key
		_, err := p.client.CopyObject(context.Background(), &s3.CopyObjectInput{
			Bucket:     &p.cfg.Bucket,
			CopySource: &copySource,
			Key:        &backupKey,
			ACL:        types.ObjectCannedACLPublicRead,
		})
		if err != nil {
			return err
		}
		_, err = p.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
			Bucket: &p.cfg.Bucket,
			Key:    &key,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *s3Publisher) restorePrefix(sourcePrefix string, targetPrefix string) error {
	keys, err := p.listKeys(sourcePrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		rel := strings.TrimPrefix(key, strings.TrimRight(sourcePrefix, "/")+"/")
		targetKey := joinKey(targetPrefix, rel)
		copySource := p.cfg.Bucket + "/" + key
		_, err := p.client.CopyObject(context.Background(), &s3.CopyObjectInput{
			Bucket:     &p.cfg.Bucket,
			CopySource: &copySource,
			Key:        &targetKey,
			ACL:        types.ObjectCannedACLPublicRead,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *s3Publisher) listKeys(prefix string) ([]string, error) {
	prefix = strings.Trim(prefix, "/")
	var keys []string
	var token *string
	for {
		out, err := p.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            &p.cfg.Bucket,
			Prefix:            &prefix,
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range out.Contents {
			if item.Key == nil {
				continue
			}
			keys = append(keys, *item.Key)
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	return keys, nil
}

type sftpPublisher struct {
	cfg *configloader.PublishTargetConfig
}

func newSFTPPublisher(cfg *configloader.PublishTargetConfig) (Publisher, error) {
	return &sftpPublisher{cfg: cfg}, nil
}

func (p *sftpPublisher) Validate() error {
	if p.cfg.Host == "" || p.cfg.Username == "" || p.cfg.RemoteDir == "" {
		return fmt.Errorf("SFTP 配置不完整")
	}
	return nil
}

func (p *sftpPublisher) TestConnection() error {
	client, sftpClient, err := p.connect()
	if err != nil {
		return fmt.Errorf("SFTP 连接失败: %w", err)
	}
	defer client.Close()
	defer sftpClient.Close()
	if err := ensureSFTPDirWritable(sftpClient, p.cfg.RemoteDir, false); err != nil {
		return fmt.Errorf("远程发布目录不可用 %s: %w", p.cfg.RemoteDir, err)
	}
	if err := ensureSFTPDirWritable(sftpClient, p.cfg.RemoteBakDir, true); err != nil {
		return fmt.Errorf("远程备份目录不可用 %s: %w", p.cfg.RemoteBakDir, err)
	}
	testFile := pathJoin(p.cfg.RemoteDir, ".cms_sftp_write_test_"+time.Now().Format("20060102_150405"))
	writer, err := sftpClient.Create(testFile)
	if err != nil {
		return fmt.Errorf("远程发布目录无写权限 %s: %w", p.cfg.RemoteDir, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("远程发布目录测试写入失败 %s: %w", testFile, err)
	}
	_ = sftpClient.Remove(testFile)
	return nil
}

func (p *sftpPublisher) PublishDir(localDir string, options PublishOptions) (*PublishResult, error) {
	client, sftpClient, err := p.connect()
	if err != nil {
		return nil, fmt.Errorf("SFTP 连接失败: %w", err)
	}
	defer client.Close()
	defer sftpClient.Close()
	emitProgress(options, "[PUBLISH] 已连接 SFTP，扫描待发布目录: %s", localDir)
	files, err := collectLocalFiles(localDir)
	if err != nil {
		return nil, err
	}
	emitProgress(options, "[PUBLISH] 已收集 %d 个文件，准备上传到 SFTP", len(files))
	remoteRoot := pathJoin(p.cfg.RemoteDir, options.TargetPath)
	result := &PublishResult{}
	if options.Mode == "overwrite" {
		emitProgress(options, "[PUBLISH] 覆盖模式开启，准备备份远程目录")
		version := time.Now().Format("20060102_150405")
		result.BackupCreatedAt = time.Now().Format("2006-01-02 15:04:05")
		result.BackupPath = pathJoin(p.cfg.RemoteBakDir, version, options.TargetPath)
		for _, entry := range topLevelEntries(files) {
			sourcePath := pathJoin(remoteRoot, entry)
			if _, err := sftpClient.Stat(sourcePath); err != nil {
				continue
			}
			targetPath := pathJoin(result.BackupPath, entry)
			emitProgress(options, "[PUBLISH] 备份远程目录项: %s -> %s", sourcePath, targetPath)
			if err := sftpClient.MkdirAll(pathDir(targetPath)); err != nil {
				return nil, fmt.Errorf("创建远程备份目录失败 %s: %w", pathDir(targetPath), err)
			}
			if err := sftpClient.Rename(sourcePath, targetPath); err != nil {
				return nil, fmt.Errorf("远程备份移动失败 %s -> %s: %w", sourcePath, targetPath, err)
			}
		}
	}
	if err := sftpClient.MkdirAll(remoteRoot); err != nil {
		return nil, fmt.Errorf("创建远程发布目录失败 %s: %w", remoteRoot, err)
	}
	for index, file := range files {
		targetPath := pathJoin(remoteRoot, file.RelPath)
		emitProgress(options, "[PUBLISH] 上传文件 %d/%d: %s -> %s", index+1, len(files), file.RelPath, targetPath)
		if err := sftpClient.MkdirAll(pathDir(targetPath)); err != nil {
			return nil, fmt.Errorf("创建远程子目录失败 %s: %w", pathDir(targetPath), err)
		}
		writer, err := sftpClient.Create(targetPath)
		if err != nil {
			return nil, fmt.Errorf("创建远程文件失败 %s: %w", targetPath, err)
		}
		if _, err := io.Copy(writer, bytes.NewReader(file.Data)); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("写入远程文件失败 %s: %w", targetPath, err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("关闭远程文件失败 %s: %w", targetPath, err)
		}
		result.Files = append(result.Files, configloader.PublishedFileRecord{
			SourceRelPath: file.RelPath,
			TargetPath:    targetPath,
			FileSize:      file.Size,
			Checksum:      file.Checksum,
		})
	}
	emitProgress(options, "[PUBLISH] SFTP 发布完成，共上传 %d 个文件", len(files))
	result.PublicURL = remoteRoot
	return result, nil
}

func (p *sftpPublisher) RestoreBackup(record *configloader.PublishRecord) error {
	client, sftpClient, err := p.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	defer sftpClient.Close()
	entries, err := sftpClient.ReadDir(record.BackupPath)
	if err != nil {
		return err
	}
	targetRoot := pathJoin(p.cfg.RemoteDir, scopePath(record))
	for _, entry := range entries {
		current := pathJoin(targetRoot, entry.Name())
		if _, err := sftpClient.Stat(current); err == nil {
			archived := current + ".restore_old_" + time.Now().Format("20060102_150405")
			if err := sftpClient.Rename(current, archived); err != nil {
				return err
			}
		}
		if err := sftpClient.MkdirAll(pathDir(current)); err != nil {
			return err
		}
		if err := sftpClient.Rename(pathJoin(record.BackupPath, entry.Name()), current); err != nil {
			return err
		}
	}
	return nil
}

func (p *sftpPublisher) connect() (*ssh.Client, *sftp.Client, error) {
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	authMethods := []ssh.AuthMethod{}
	if p.cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(p.cfg.Password))
	}
	if p.cfg.PrivateKeyPath != "" {
		raw, err := os.ReadFile(p.cfg.PrivateKeyPath)
		if err != nil {
			return nil, nil, err
		}
		signer, err := ssh.ParsePrivateKey(raw)
		if err != nil {
			return nil, nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	sshConfig := &ssh.ClientConfig{
		User:            p.cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	address := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, nil, err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	return client, sftpClient, nil
}

func joinKey(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

func pathJoin(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ReplaceAll(part, "\\", "/")
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return "/" + strings.Join(cleaned, "/")
}

func pathDir(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	index := strings.LastIndex(path, "/")
	if index <= 0 {
		return "/"
	}
	return path[:index]
}

func ensureSFTPDirWritable(client *sftp.Client, target string, createIfMissing bool) error {
	if target == "" {
		return fmt.Errorf("目录不能为空")
	}
	if _, err := client.Stat(target); err == nil {
		return nil
	}
	if !createIfMissing {
		return fmt.Errorf("目录不存在或不可访问")
	}
	if err := client.MkdirAll(target); err != nil {
		return err
	}
	return nil
}

func scopePath(record *configloader.PublishRecord) string {
	if record.PublishingScope == "single_book" && len(record.BuildBooks) == 1 {
		return record.BuildBooks[0]
	}
	return ""
}
