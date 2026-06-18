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
	HTTPPort          int         `yaml:"http_port"`
	SourceRootPath    string      `yaml:"source_root_path"`
	HugoBinPath       string      `yaml:"hugo_bin_path"`
	GlobalThemePath   string      `yaml:"global_theme_path"`
	BuildTempRoot     string      `yaml:"build_temp_root"`
	PublishRecordPath string      `yaml:"publish_record_path"`
	TempCleanInterval int         `yaml:"temp_clean_interval"`
	BuildTaskTimeout  int         `yaml:"build_task_timeout"`
	Auth              AuthConfig  `yaml:"auth"`
	S3                S3Config    `yaml:"s3"`
	EditorLimit       EditorLimit `yaml:"editor_limit"`
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
	RecordID       string   `yaml:"record_id"`
	PublishingTime string   `yaml:"publishing_time"`
	PublishingType string   `yaml:"publishing_type"`
	BuildBooks     []string `yaml:"build_books"`
	TempOutputPath string   `yaml:"temp_output_path"`
	S3Bucket       string   `yaml:"s3_bucket"`
	S3Prefix       string   `yaml:"s3_prefix"`
	PublicURL      string   `yaml:"public_url"`
	FullLog        string   `yaml:"full_log"`
	Status         string   `yaml:"status"`
	ErrorMsg       string   `yaml:"error_msg"`
}

func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		HTTPPort:          8080,
		SourceRootPath:    "./source_root",
		HugoBinPath:       "./bin/hugo-extended",
		GlobalThemePath:   "./global_theme",
		BuildTempRoot:     "./build_temp",
		PublishRecordPath: "./publish_records",
		TempCleanInterval: 24,
		BuildTaskTimeout:  300,
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
	if cfg.SourceRootPath == "" || cfg.HugoBinPath == "" || cfg.BuildTempRoot == "" || cfg.PublishRecordPath == "" {
		return fmt.Errorf("基础路径配置不能为空")
	}
	return nil
}
