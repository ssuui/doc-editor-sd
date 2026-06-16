package hugobuilder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"doc-publish-server/internal/configloader"
)

type Service struct {
	cfg *configloader.SystemConfig
}

func New(cfg *configloader.SystemConfig) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) CheckBinary() error {
	info, err := os.Stat(s.cfg.HugoBinPath)
	if err != nil {
		return err
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("hugo 二进制缺少执行权限: %s", s.cfg.HugoBinPath)
	}
	return nil
}

func (s *Service) BuildMainSite(sourceRoot string) (string, error) {
	out := filepath.Join(s.cfg.BuildTempRoot, "main_site_out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	buildSource := filepath.Join(s.cfg.BuildTempRoot, "main_site_src")
	if err := os.RemoveAll(buildSource); err != nil {
		return "", err
	}
	if err := os.MkdirAll(buildSource, 0o755); err != nil {
		return "", err
	}
	if err := copyDir(filepath.Join(sourceRoot, "global_static"), filepath.Join(buildSource, "static", "global_static")); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	contentDir := filepath.Join(buildSource, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return "", err
	}
	indexPath := filepath.Join(sourceRoot, "index.md")
	targetIndex := filepath.Join(contentDir, "_index.md")
	if _, err := os.Stat(indexPath); err == nil {
		raw, readErr := os.ReadFile(indexPath)
		if readErr != nil {
			return "", readErr
		}
		if err := os.WriteFile(targetIndex, raw, 0o644); err != nil {
			return "", err
		}
	} else {
		generated, genErr := s.generatePortalIndex(sourceRoot)
		if genErr != nil {
			return "", genErr
		}
		if err := os.WriteFile(targetIndex, []byte(generated), 0o644); err != nil {
			return "", err
		}
	}
	configRaw := []byte("baseURL = \"/\"\nlanguageCode = \"zh-cn\"\ntitle = \"文档门户\"\n")
	if err := os.WriteFile(filepath.Join(buildSource, "hugo.toml"), configRaw, 0o644); err != nil {
		return "", err
	}
	return s.runCommand([]string{
		"--source=" + buildSource,
		"--themesDir=" + s.cfg.GlobalThemePath,
		"--theme=main-site-template",
		"--destination=" + out,
		"--timeout=" + fmt.Sprintf("%ds", s.cfg.BuildTaskTimeout),
	})
}

func (s *Service) BuildBook(sourceRoot string, bookDir string) (string, error) {
	bookSource := filepath.Join(sourceRoot, bookDir)
	out := filepath.Join(s.cfg.BuildTempRoot, "book_cache", bookDir)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	return s.runCommand([]string{
		"--source=" + bookSource,
		"--themesDir=" + s.cfg.GlobalThemePath,
		"--theme=book-site-template",
		"--destination=" + out,
		"--timeout=" + fmt.Sprintf("%ds", s.cfg.BuildTaskTimeout),
	})
}

func (s *Service) MergeFullPackage(bookDirs []string) error {
	full := filepath.Join(s.cfg.BuildTempRoot, "full_package")
	_ = os.RemoveAll(full)
	if err := os.MkdirAll(full, 0o755); err != nil {
		return err
	}
	if err := copyDir(filepath.Join(s.cfg.BuildTempRoot, "main_site_out"), full); err != nil {
		return err
	}
	for _, bookDir := range bookDirs {
		if err := copyDir(filepath.Join(s.cfg.BuildTempRoot, "book_cache", bookDir), filepath.Join(full, bookDir)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runCommand(args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.BuildTaskTimeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.HugoBinPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	logs := out.String()
	if ctx.Err() == context.DeadlineExceeded {
		return logs, fmt.Errorf("hugo build timeout")
	}
	if err != nil {
		return logs, err
	}
	return logs, nil
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode())
	})
}

func JoinLogs(parts ...string) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			lines = append(lines, strings.TrimSpace(part))
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Service) generatePortalIndex(sourceRoot string) (string, error) {
	siteMeta, err := configloader.LoadSiteMeta(filepath.Join(sourceRoot, "_site_meta.yaml"))
	if err != nil {
		return "", err
	}
	sort.Slice(siteMeta.BookList, func(i, j int) bool {
		return siteMeta.BookList[i].Weight < siteMeta.BookList[j].Weight
	})
	lines := []string{
		"# 企业文档门户",
		"",
		"以下内容由系统根据书籍元数据自动生成。",
		"",
		"## 书籍导航",
		"",
	}
	for _, item := range siteMeta.BookList {
		if !item.EnableHomeShow {
			continue
		}
		meta, err := configloader.LoadBookMeta(filepath.Join(sourceRoot, item.BookDirName, "book_meta.yaml"))
		if err != nil {
			return "", err
		}
		lines = append(lines,
			fmt.Sprintf("### [%s](/%s/)", meta.DisplayName, item.BookDirName),
			"",
			meta.Description,
			"",
			fmt.Sprintf("- 目录：`%s`", item.BookDirName),
			fmt.Sprintf("- 版本：`%s`", meta.Version),
			fmt.Sprintf("- 标签：%s", strings.Join(meta.Tags, " / ")),
			"",
		)
	}
	return strings.Join(lines, "\n"), nil
}
