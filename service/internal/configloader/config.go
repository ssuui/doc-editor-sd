package configloader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type SystemConfig struct {
	HTTPPort           int         `yaml:"http_port"`
	SourceRootPath     string      `yaml:"source_root_path"`
	HugoBinPath        string      `yaml:"hugo_bin_path"`
	GlobalThemePath    string      `yaml:"global_theme_path"`
	BuildTempRoot      string      `yaml:"build_temp_root"`
	PublishRecordPath  string      `yaml:"publish_record_path"`
	PublishTargetsPath string      `yaml:"publish_targets_path"`
	TempCleanInterval  int         `yaml:"temp_clean_interval"`
	BuildTaskTimeout   int         `yaml:"build_task_timeout"`
	Auth               AuthConfig  `yaml:"auth"`
	S3                 S3Config    `yaml:"s3"`
	EditorLimit        EditorLimit `yaml:"editor_limit"`
}

type AuthConfig struct {
	AdminUsername    string `yaml:"admin_username"`
	AdminPassword    string `yaml:"admin_password"`
	TokenExpireHours int64  `yaml:"token_expire_hours"`
}

type S3Config struct {
	Endpoint            string `yaml:"endpoint"`
	AccessKeyID         string `yaml:"access_key_id"`
	SecretAccessKey     string `yaml:"secret_access_key"`
	DefaultBucketName   string `yaml:"default_bucket_name"`
	Region              string `yaml:"region"`
	SitePublicDomain    string `yaml:"site_public_domain"`
	ImgCDNDomain        string `yaml:"img_cdn_domain"`
	ImgStorePrefix      string `yaml:"img_store_prefix"`
	PresignPutExpireMin int64  `yaml:"presign_put_expire_min"`
	CacheHTML           string `yaml:"cache_html"`
	CacheStatic         string `yaml:"cache_static"`
}

type EditorLimit struct {
	MaxFileMB   int      `yaml:"max_file_mb"`
	AllowImgExt []string `yaml:"allow_img_ext"`
}

type SiteGlobalConfig struct {
	SiteTitle       string         `yaml:"site_title"`
	SiteLogo        string         `yaml:"site_logo"`
	FooterText      string         `yaml:"footer_text"`
	FooterCopyright string         `yaml:"footer_copyright"`
	ICP             ICPConfig      `yaml:"icp"`
	HomeNoticeText  string         `yaml:"home_notice_text"`
	BooksPagePath   string         `yaml:"books_page_path"`
	BooksPageTitle  string         `yaml:"books_page_title"`
	HomeCardLayout  HomeCardLayout `yaml:"home_card_layout"`
	GlobalNav       []NavItem      `yaml:"global_nav"`
	CustomGlobalCSS string         `yaml:"custom_global_css"`
	EnableSitemap   bool           `yaml:"enable_sitemap"`
	EnableRSS       bool           `yaml:"enable_rss"`
}

func (c *SiteGlobalConfig) EffectiveSiteTitle() string {
	title := strings.TrimSpace(c.SiteTitle)
	if title == "" {
		return "知识库"
	}
	return title
}

func (c *SiteGlobalConfig) AdminTitle() string {
	return c.EffectiveSiteTitle() + "发布器"
}

type ICPConfig struct {
	Number string `yaml:"number"`
	Link   string `yaml:"link"`
}

type HomeCardLayout struct {
	ColumnCount    int  `yaml:"column_count"`
	ShowBookDesc   bool `yaml:"show_book_desc"`
	ShowVersionTag bool `yaml:"show_version_tag"`
}

type NavItem struct {
	Name string `yaml:"name"`
	Link string `yaml:"link"`
}

type SiteMeta struct {
	BookList []SiteBookItem `yaml:"book_list"`
}

type SiteBookItem struct {
	BookDirName    string `yaml:"book_dir_name"`
	Weight         int    `yaml:"weight"`
	EnableHomeShow bool   `yaml:"enable_home_show"`
}

type BooksMeta struct {
	BookList []BooksBookItem `yaml:"book_list"`
}

type BooksBookItem struct {
	BookDirName string `yaml:"book_dir_name"`
	Weight      int    `yaml:"weight"`
}

type BookMeta struct {
	DisplayName   string        `yaml:"display_name"`
	CoverImg      string        `yaml:"cover_img"`
	Description   string        `yaml:"description"`
	Tags          []string      `yaml:"tags"`
	Version       string        `yaml:"version"`
	VisibleInHome bool          `yaml:"visible_in_home"`
	ExtraNavLinks []BookNavLink `yaml:"extra_nav_links"`
	SidebarOrder  []string      `yaml:"sidebar_order"`
}

type BookNavLink struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type PublishRecord struct {
	RecordID             string                `yaml:"record_id" json:"record_id"`
	PublishingTime       string                `yaml:"publishing_time" json:"publishing_time"`
	PublishingType       string                `yaml:"publishing_type" json:"publishing_type"`
	PublishingScope      string                `yaml:"publishing_scope" json:"publishing_scope"`
	PublishMode          string                `yaml:"publish_mode" json:"publish_mode"`
	PublishingTargetType string                `yaml:"publishing_target_type" json:"publishing_target_type"`
	PublishingTargetID   string                `yaml:"publishing_target_id" json:"publishing_target_id"`
	PublishingTargetName string                `yaml:"publishing_target_name" json:"publishing_target_name"`
	BuildBooks           []string              `yaml:"build_books" json:"build_books"`
	TempOutputPath       string                `yaml:"temp_output_path" json:"temp_output_path"`
	S3Bucket             string                `yaml:"s3_bucket" json:"s3_bucket"`
	S3Prefix             string                `yaml:"s3_prefix" json:"s3_prefix"`
	PublicURL            string                `yaml:"public_url" json:"public_url"`
	FullLog              string                `yaml:"full_log" json:"full_log"`
	Status               string                `yaml:"status" json:"status"`
	ErrorMsg             string                `yaml:"error_msg" json:"error_msg"`
	BackupPath           string                `yaml:"backup_path" json:"backup_path"`
	BackupCreatedAt      string                `yaml:"backup_created_at" json:"backup_created_at"`
	TargetConfigSnapshot map[string]any        `yaml:"target_config_snapshot,omitempty" json:"target_config_snapshot,omitempty"`
	PublishedFiles       []PublishedFileRecord `yaml:"published_files,omitempty" json:"published_files,omitempty"`
}

type PublishedFileRecord struct {
	SourceRelPath string `yaml:"source_rel_path" json:"source_rel_path"`
	TargetPath    string `yaml:"target_path" json:"target_path"`
	TargetKey     string `yaml:"target_key,omitempty" json:"target_key,omitempty"`
	FileSize      int64  `yaml:"file_size" json:"file_size"`
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

type PublishTargetConfig struct {
	ID               string `yaml:"id" json:"id"`
	Name             string `yaml:"name" json:"name"`
	Type             string `yaml:"type" json:"type"`
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	CreatedAt        string `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        string `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
	ModeDefault      string `yaml:"mode_default,omitempty" json:"mode_default,omitempty"`
	Bucket           string `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	Region           string `yaml:"region,omitempty" json:"region,omitempty"`
	Endpoint         string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	AccessKeyID      string `yaml:"access_key_id,omitempty" json:"access_key_id,omitempty"`
	SecretAccessKey  string `yaml:"secret_access_key,omitempty" json:"secret_access_key,omitempty"`
	SitePublicDomain string `yaml:"site_public_domain,omitempty" json:"site_public_domain,omitempty"`
	BasePrefix       string `yaml:"base_prefix,omitempty" json:"base_prefix,omitempty"`
	CacheHTML        string `yaml:"cache_html,omitempty" json:"cache_html,omitempty"`
	CacheStatic      string `yaml:"cache_static,omitempty" json:"cache_static,omitempty"`
	TargetDir        string `yaml:"target_dir,omitempty" json:"target_dir,omitempty"`
	BakDir           string `yaml:"bak_dir,omitempty" json:"bak_dir,omitempty"`
	Host             string `yaml:"host,omitempty" json:"host,omitempty"`
	Port             int    `yaml:"port,omitempty" json:"port,omitempty"`
	Username         string `yaml:"username,omitempty" json:"username,omitempty"`
	Password         string `yaml:"password,omitempty" json:"password,omitempty"`
	PrivateKeyPath   string `yaml:"private_key_path,omitempty" json:"private_key_path,omitempty"`
	RemoteDir        string `yaml:"remote_dir,omitempty" json:"remote_dir,omitempty"`
	RemoteBakDir     string `yaml:"remote_bak_dir,omitempty" json:"remote_bak_dir,omitempty"`
}

func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		HTTPPort:           8080,
		SourceRootPath:     "./source_root",
		HugoBinPath:        "./bin/hugo-extended",
		GlobalThemePath:    "./global_theme",
		BuildTempRoot:      "./build_temp",
		PublishRecordPath:  "./publish_records",
		PublishTargetsPath: "./config/publish_targets",
		TempCleanInterval:  24,
		BuildTaskTimeout:   300,
		Auth: AuthConfig{
			AdminUsername:    "admin",
			AdminPassword:    "AtlasDocs_2026!",
			TokenExpireHours: 360,
		},
		S3: S3Config{
			DefaultBucketName:   "atlas-doc-portal-1300012345",
			SitePublicDomain:    "docs.atlaslab.example",
			ImgCDNDomain:        "assets.atlaslab.example",
			ImgStorePrefix:      "book-res/",
			PresignPutExpireMin: 15,
			CacheHTML:           "max-age=600",
			CacheStatic:         "max-age=86400",
		},
		EditorLimit: EditorLimit{
			MaxFileMB:   20,
			AllowImgExt: []string{".png", ".jpg", ".jpeg", ".gif", ".webp"},
		},
	}
}

func DefaultSiteGlobalConfig() *SiteGlobalConfig {
	return &SiteGlobalConfig{
		SiteTitle:       "企业内部技术文档门户",
		SiteLogo:        "/global_static/logo.svg",
		FooterText:      "文档自动发布系统 | Hugo静态生成 + S3静态托管",
		FooterCopyright: "",
		ICP:             ICPConfig{},
		HomeNoticeText:  "文档门户已切换为 Hextra 主题，支持全文搜索、暗色模式和更清晰的目录导航。",
		BooksPagePath:   "/books/",
		BooksPageTitle:  "全部书籍",
		HomeCardLayout: HomeCardLayout{
			ColumnCount:    3,
			ShowBookDesc:   true,
			ShowVersionTag: true,
		},
		GlobalNav: []NavItem{
			{Name: "门户首页", Link: "/"},
			{Name: "全部书籍", Link: "/books/index.html"},
		},
		EnableSitemap: true,
		EnableRSS:     false,
	}
}

func LoadSystemConfig(path string) (*SystemConfig, error) {
	cfg := DefaultSystemConfig()
	if err := ensureYAML(path, cfg); err != nil {
		return nil, err
	}
	if err := loadYAML(path, cfg); err != nil {
		return nil, err
	}
	return cfg, validateSystemConfig(cfg)
}

func LoadSiteGlobalConfig(path string) (*SiteGlobalConfig, error) {
	cfg := DefaultSiteGlobalConfig()
	if err := ensureYAML(path, cfg); err != nil {
		return nil, err
	}
	return cfg, loadYAML(path, cfg)
}

func LoadSiteMeta(path string) (*SiteMeta, error) {
	meta := &SiteMeta{}
	if err := loadYAML(path, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func LoadBooksMeta(path string) (*BooksMeta, error) {
	meta := &BooksMeta{}
	if err := loadYAML(path, meta); err == nil {
		return meta, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	siteMeta, err := LoadSiteMeta(filepath.Join(filepath.Dir(path), "_site_meta.yaml"))
	if err != nil {
		return nil, err
	}

	meta.BookList = make([]BooksBookItem, 0, len(siteMeta.BookList))
	for _, item := range siteMeta.BookList {
		meta.BookList = append(meta.BookList, BooksBookItem{
			BookDirName: item.BookDirName,
			Weight:      item.Weight,
		})
	}
	return meta, nil
}

func LoadBookMeta(path string) (*BookMeta, error) {
	meta := &BookMeta{}
	if err := loadYAML(path, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func SaveYAML(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func ensureYAML(path string, data any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return SaveYAML(path, data)
}

func loadYAML(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, target)
}

func validateSystemConfig(cfg *SystemConfig) error {
	if cfg.Auth.AdminUsername == "" || cfg.Auth.AdminPassword == "" {
		return errors.New("auth.admin_username/admin_password 不能为空")
	}
	if cfg.Auth.TokenExpireHours <= 0 {
		return errors.New("auth.token_expire_hours 必须大于 0")
	}
	if cfg.S3.Endpoint == "" || cfg.S3.AccessKeyID == "" || cfg.S3.SecretAccessKey == "" || cfg.S3.Region == "" {
		return errors.New("s3.endpoint/access_key_id/secret_access_key/region 不能为空")
	}
	if cfg.S3.ImgCDNDomain == "" || cfg.S3.ImgStorePrefix == "" || cfg.S3.PresignPutExpireMin <= 0 {
		return errors.New("s3.img_cdn_domain/img_store_prefix/presign_put_expire_min 不能为空")
	}
	if cfg.SourceRootPath == "" || cfg.HugoBinPath == "" || cfg.BuildTempRoot == "" || cfg.PublishRecordPath == "" || cfg.PublishTargetsPath == "" {
		return fmt.Errorf("基础路径配置不能为空")
	}
	return nil
}
