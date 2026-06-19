package publishtarget

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"doc-publish-server/internal/configloader"

	"gopkg.in/yaml.v3"
)

type Service struct {
	root string
}

func New(root string) *Service {
	return &Service{root: filepath.Clean(root)}
}

func (s *Service) List() ([]configloader.PublishTargetConfig, error) {
	configs := make([]configloader.PublishTargetConfig, 0)
	for _, kind := range []string{"s3", "local", "sftp"} {
		dir := filepath.Join(s.root, kind)
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			cfg, err := s.loadFromFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			configs = append(configs, *cfg)
		}
	}
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Type == configs[j].Type {
			return configs[i].Name < configs[j].Name
		}
		return configs[i].Type < configs[j].Type
	})
	return configs, nil
}

func (s *Service) Detail(targetType string, id string) (*configloader.PublishTargetConfig, error) {
	return s.loadFromFile(s.filePath(targetType, id))
}

func (s *Service) Save(cfg *configloader.PublishTargetConfig) error {
	if _, err := validate(cfg); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if strings.TrimSpace(cfg.CreatedAt) == "" {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	if err := os.MkdirAll(filepath.Dir(s.filePath(cfg.Type, cfg.ID)), 0o755); err != nil {
		return err
	}
	return configloader.SaveYAML(s.filePath(cfg.Type, cfg.ID), cfg)
}

func (s *Service) Remove(targetType string, id string) error {
	return os.Remove(s.filePath(targetType, id))
}

func (s *Service) filePath(targetType string, id string) string {
	return filepath.Join(s.root, typeDir(targetType), id+".yaml")
}

func (s *Service) loadFromFile(path string) (*configloader.PublishTargetConfig, error) {
	cfg := &configloader.PublishTargetConfig{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yamlUnmarshal(raw, cfg); err != nil {
		return nil, err
	}
	return validate(cfg)
}

func yamlUnmarshal(data []byte, target any) error {
	if err := yaml.Unmarshal(data, target); err != nil {
		return err
	}
	return nil
}

func typeDir(targetType string) string {
	switch targetType {
	case "s3":
		return "s3"
	case "local_dir":
		return "local"
	case "sftp":
		return "sftp"
	default:
		return targetType
	}
}

func validate(cfg *configloader.PublishTargetConfig) (*configloader.PublishTargetConfig, error) {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Type = strings.TrimSpace(cfg.Type)
	if cfg.ID == "" || cfg.Name == "" || cfg.Type == "" {
		return nil, fmt.Errorf("发布配置 id/name/type 不能为空")
	}
	if strings.Contains(cfg.ID, "/") || strings.Contains(cfg.ID, "\\") || strings.Contains(cfg.ID, "..") {
		return nil, fmt.Errorf("发布配置 id 不能包含路径分隔符或 ..")
	}
	if cfg.ModeDefault == "" {
		cfg.ModeDefault = "incremental"
	}
	switch cfg.Type {
	case "s3":
		if cfg.Bucket == "" || cfg.Region == "" || cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return nil, fmt.Errorf("S3 发布配置不完整")
		}
	case "local_dir":
		if cfg.TargetDir == "" {
			return nil, fmt.Errorf("本地目录发布 target_dir 不能为空")
		}
		if cfg.BakDir == "" {
			cfg.BakDir = filepath.Join(cfg.TargetDir, "bak")
		}
	case "sftp":
		if cfg.Host == "" || cfg.Username == "" || cfg.RemoteDir == "" {
			return nil, fmt.Errorf("SFTP 发布配置不完整")
		}
		if cfg.Port == 0 {
			cfg.Port = 22
		}
		if cfg.Password == "" && cfg.PrivateKeyPath == "" {
			return nil, fmt.Errorf("SFTP 需要 password 或 private_key_path")
		}
		if cfg.RemoteBakDir == "" {
			cfg.RemoteBakDir = strings.TrimRight(cfg.RemoteDir, "/") + "/bak"
		}
	default:
		return nil, fmt.Errorf("未知发布类型: %s", cfg.Type)
	}
	return cfg, nil
}
