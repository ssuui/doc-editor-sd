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
	cfg     *configloader.SystemConfig
	siteCfg *configloader.SiteGlobalConfig
}

func New(cfg *configloader.SystemConfig, siteCfg *configloader.SiteGlobalConfig) *Service {
	return &Service{cfg: cfg, siteCfg: siteCfg}
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
	if err := os.RemoveAll(out); err != nil {
		return "", err
	}
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
	if err := s.writeBooksPage(sourceRoot, contentDir); err != nil {
		return "", err
	}
	if err := s.prepareMarkdownTree(contentDir, markdownPrepOptions{
		RootTitle:    s.siteCfg.SiteTitle,
		RootHasCards: true,
	}); err != nil {
		return "", err
	}
	configRaw := []byte(s.buildMainSiteConfig())
	if err := os.WriteFile(filepath.Join(buildSource, "hugo.toml"), configRaw, 0o644); err != nil {
		return "", err
	}
	absBuildSource, err := filepath.Abs(buildSource)
	if err != nil {
		return "", err
	}
	absThemeDir, err := filepath.Abs(s.cfg.GlobalThemePath)
	if err != nil {
		return "", err
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	return s.runCommand([]string{
		"--source=" + absBuildSource,
		"--themesDir=" + absThemeDir,
		"--theme=main-site-template",
		"--destination=" + absOut,
	})
}

func (s *Service) BuildBook(sourceRoot string, bookDir string) (string, error) {
	bookSource := filepath.Join(sourceRoot, bookDir)
	buildSource := filepath.Join(s.cfg.BuildTempRoot, "book_site_src", bookDir)
	out := filepath.Join(s.cfg.BuildTempRoot, "book_cache", bookDir)
	if err := os.RemoveAll(buildSource); err != nil {
		return "", err
	}
	if err := os.RemoveAll(out); err != nil {
		return "", err
	}
	if err := copyDir(bookSource, buildSource); err != nil {
		return "", err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	bookMeta, err := configloader.LoadBookMeta(filepath.Join(buildSource, "book_meta.yaml"))
	if err != nil {
		return "", err
	}
	if err := s.prepareMarkdownTree(filepath.Join(buildSource, "content"), markdownPrepOptions{
		RootTitle:       bookMeta.DisplayName,
		RootCascadeDocs: true,
	}); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(buildSource, "hugo.toml"), []byte(s.buildBookConfig(bookDir, bookMeta)), 0o644); err != nil {
		return "", err
	}
	absBookSource, err := filepath.Abs(buildSource)
	if err != nil {
		return "", err
	}
	absThemeDir, err := filepath.Abs(s.cfg.GlobalThemePath)
	if err != nil {
		return "", err
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	return s.runCommand([]string{
		"--source=" + absBookSource,
		"--themesDir=" + absThemeDir,
		"--theme=book-site-template",
		"--destination=" + absOut,
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
	return s.generateBookListPage(sourceRoot, s.siteCfg.SiteTitle, true)
}

func (s *Service) writeBooksPage(sourceRoot string, contentDir string) error {
	booksDir := filepath.Join(contentDir, booksPageSectionPath(s.siteCfg.BooksPagePath))
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		return err
	}
	generated, err := s.generateBookListPage(sourceRoot, s.booksPageTitle(), false)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(booksDir, "_index.md"), []byte(generated), 0o644)
}

func (s *Service) generateBookListPage(sourceRoot string, title string, includeHomeNotice bool) (string, error) {
	siteMeta, err := configloader.LoadSiteMeta(filepath.Join(sourceRoot, "_site_meta.yaml"))
	if err != nil {
		return "", err
	}
	sort.Slice(siteMeta.BookList, func(i, j int) bool {
		return siteMeta.BookList[i].Weight < siteMeta.BookList[j].Weight
	})
	lines := []string{
		"# " + title,
		"",
		"<div id=\"book-list\"></div>",
		"",
		"## 文档总览",
		"",
	}
	if includeHomeNotice && strings.TrimSpace(s.siteCfg.HomeNoticeText) != "" {
		lines = append(lines,
			"{{< callout type=\"info\" >}}",
			s.siteCfg.HomeNoticeText,
			"{{< /callout >}}",
			"",
		)
	}
	lines = append(lines,
		"{{< cards cols=\"2\" >}}",
	)
	for _, item := range siteMeta.BookList {
		if !item.EnableHomeShow {
			continue
		}
		meta, err := configloader.LoadBookMeta(filepath.Join(sourceRoot, item.BookDirName, "book_meta.yaml"))
		if err != nil {
			return "", err
		}
		subtitle := meta.Description
		tag := meta.Version
		if tag == "" {
			tag = item.BookDirName
		}
		if len(meta.Tags) > 0 {
			subtitle = subtitle + "  标签：" + strings.Join(meta.Tags, " / ")
		}
		lines = append(lines,
			fmt.Sprintf("{{< card link=\"/%s/index.html\" title=\"%s\" subtitle=\"%s\" icon=\"book-open\" tag=\"%s\" >}}", item.BookDirName, escapeFrontMatterString(meta.DisplayName), escapeFrontMatterString(subtitle), escapeFrontMatterString(tag)),
		)
	}
	lines = append(lines,
		"{{< /cards >}}",
	)
	return strings.Join(lines, "\n"), nil
}

func (s *Service) booksPageTitle() string {
	if strings.TrimSpace(s.siteCfg.BooksPageTitle) != "" {
		return strings.TrimSpace(s.siteCfg.BooksPageTitle)
	}
	return "全部书籍"
}

func (s *Service) siteLogoPath() string {
	if strings.TrimSpace(s.siteCfg.SiteLogo) != "" {
		return strings.TrimSpace(s.siteCfg.SiteLogo)
	}
	return "/global_static/logo.svg"
}

func (s *Service) mainSiteURL() string {
	domain := strings.TrimSpace(s.cfg.S3.SitePublicDomain)
	if domain == "" {
		return "/"
	}
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return strings.TrimRight(domain, "/") + "/"
	}
	return "https://" + strings.TrimRight(domain, "/") + "/"
}

func booksPageSectionPath(configPath string) string {
	cleaned := strings.TrimSpace(configPath)
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "" {
		return "books"
	}
	return cleaned
}

type markdownPrepOptions struct {
	RootTitle       string
	RootCascadeDocs bool
	RootHasCards    bool
}

func (s *Service) prepareMarkdownTree(contentDir string, opts markdownPrepOptions) error {
	return filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, relErr := filepath.Rel(contentDir, path)
		if relErr != nil {
			return relErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		updated := normalizeMarkdown(raw, rel, opts)
		return os.WriteFile(path, updated, 0o644)
	})
}

func normalizeMarkdown(raw []byte, rel string, opts markdownPrepOptions) []byte {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if hasFrontMatter(text) {
		return []byte(text)
	}
	title, body := extractLeadingH1(text)
	if rel == "_index.md" && opts.RootTitle != "" {
		title = opts.RootTitle
	}
	if title == "" {
		title = fallbackTitleFromPath(rel)
	}
	frontMatter := make([]string, 0, 8)
	frontMatter = append(frontMatter, "---")
	frontMatter = append(frontMatter, fmt.Sprintf("title: \"%s\"", escapeFrontMatterString(title)))
	if rel == "_index.md" && opts.RootCascadeDocs {
		frontMatter = append(frontMatter, "cascade:")
		frontMatter = append(frontMatter, "  type: docs")
	}
	frontMatter = append(frontMatter, "---", "")
	body = strings.TrimLeft(body, "\n")
	if rel == "_index.md" && opts.RootHasCards {
		body = strings.TrimLeft(body, "\n")
	}
	if body != "" {
		return []byte(strings.Join(frontMatter, "\n") + body + ensureTrailingNewline(body))
	}
	return []byte(strings.Join(frontMatter, "\n"))
}

func extractLeadingH1(text string) (string, string) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			body := strings.Join(lines[i+1:], "\n")
			return title, body
		}
		break
	}
	return "", text
}

func hasFrontMatter(text string) bool {
	return strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "+++\n")
}

func fallbackTitleFromPath(rel string) string {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if base == "_index" {
		base = filepath.Base(filepath.Dir(rel))
	}
	base = strings.TrimLeft(base, "0123456789-_ ")
	if base == "" {
		return "未命名页面"
	}
	return base
}

func ensureTrailingNewline(body string) string {
	if strings.HasSuffix(body, "\n") {
		return ""
	}
	return "\n"
}

func escapeFrontMatterString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func (s *Service) buildMainSiteConfig() string {
	nav := make([]string, 0, len(s.siteCfg.GlobalNav)+1)
	for idx, item := range s.siteCfg.GlobalNav {
		nav = append(nav, strings.Join([]string{
			"[[menu.main]]",
			fmt.Sprintf("name = %q", item.Name),
			fmt.Sprintf("url = %q", item.Link),
			fmt.Sprintf("weight = %d", idx+1),
			"",
		}, "\n"))
	}
	nav = append(nav, strings.Join([]string{
		"[[menu.main]]",
		`name = "搜索"`,
		fmt.Sprintf("weight = %d", len(s.siteCfg.GlobalNav)+1),
		"[menu.main.params]",
		`type = "search"`,
		"",
	}, "\n"))
	return strings.TrimSpace(fmt.Sprintf(`
baseURL = "/"
title = %q
defaultContentLanguage = "zh-cn"
hasCJKLanguage = true
enableRobotsTXT = true
enableInlineShortcodes = true
disableKinds = ["taxonomy", "term"]

[markup.highlight]
noClasses = false

[markup.goldmark.renderer]
unsafe = true

[params]
description = %q

[params.navbar]
displayTitle = true
displayLogo = true
width = "wide"

[params.navbar.logo]
path = %q
link = "/"

[params.footer]
enable = true
displayCopyright = false
displayPoweredBy = false
width = "wide"
text = %q
icpNumber = %q
icpLink = %q
copyright = %q

[params.theme]
default = "system"
displayToggle = true

[params.search]
enable = true
type = "flexsearch"

[params.page]
width = "wide"

%s`, s.siteCfg.SiteTitle, s.siteCfg.FooterText, s.siteLogoPath(), s.siteCfg.FooterText, s.siteCfg.ICP.Number, s.siteCfg.ICP.Link, s.siteCfg.FooterCopyright, strings.Join(nav, "\n")))
}

func (s *Service) buildBookConfig(bookDir string, meta *configloader.BookMeta) string {
	menuBlocks := []string{
		strings.Join([]string{
			"[[menu.main]]",
			`name = "主站"`,
			fmt.Sprintf("url = %q", s.mainSiteURL()),
			"weight = 1",
			"",
		}, "\n"),
		strings.Join([]string{
			"[[menu.main]]",
			`name = "搜索"`,
			"weight = 99",
			"[menu.main.params]",
			`type = "search"`,
			"",
		}, "\n"),
	}
	for idx, item := range meta.ExtraNavLinks {
		menuBlocks = append(menuBlocks, strings.Join([]string{
			"[[menu.main]]",
			fmt.Sprintf("name = %q", item.Name),
			fmt.Sprintf("url = %q", item.URL),
			fmt.Sprintf("weight = %d", idx+10),
			"",
		}, "\n"))
	}
	description := meta.Description
	if len(meta.Tags) > 0 {
		description = strings.TrimSpace(description + " 标签：" + strings.Join(meta.Tags, " / "))
	}
	return strings.TrimSpace(fmt.Sprintf(`
baseURL = %q
title = %q
defaultContentLanguage = "zh-cn"
hasCJKLanguage = true
enableRobotsTXT = true
enableInlineShortcodes = true
disableKinds = ["taxonomy", "term"]

[markup.highlight]
noClasses = false

[markup.goldmark.renderer]
unsafe = true

[params]
description = %q

[params.navbar]
displayTitle = true
displayLogo = false
width = "wide"

[params.footer]
enable = true
displayCopyright = false
displayPoweredBy = false
width = "wide"
text = %q
icpNumber = %q
icpLink = %q
copyright = %q

[params.theme]
default = "system"
displayToggle = true

[params.search]
enable = true
type = "flexsearch"

[params.page]
width = "normal"

[params.toc]
displayTags = false

%s
`, "/"+bookDir+"/", meta.DisplayName, description, s.siteCfg.FooterText, s.siteCfg.ICP.Number, s.siteCfg.ICP.Link, s.siteCfg.FooterCopyright, strings.Join(menuBlocks, "\n")))
}
