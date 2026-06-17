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
	"gopkg.in/yaml.v3"
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
		SidebarOrder:    bookMeta.SidebarOrder,
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
	SidebarOrder    []string
}

func (s *Service) prepareMarkdownTree(contentDir string, opts markdownPrepOptions) error {
	if err := ensureSectionIndexFiles(contentDir); err != nil {
		return err
	}
	files := make([]string, 0, 32)
	if err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
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
		files = append(files, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(files)
	weights := buildSidebarWeights(files, opts.SidebarOrder)
	for _, rel := range files {
		path := filepath.Join(contentDir, filepath.FromSlash(rel))
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		weight, hasWeight := weights[rel]
		updated := normalizeMarkdown(raw, rel, opts, weight, hasWeight)
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureSectionIndexFiles(contentDir string) error {
	return filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || path == contentDir {
			return nil
		}
		indexPath := filepath.Join(path, "_index.md")
		if _, statErr := os.Stat(indexPath); statErr == nil {
			return nil
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		hasContent := false
		for _, entry := range entries {
			if entry.IsDir() {
				hasContent = true
				break
			}
			name := entry.Name()
			if strings.EqualFold(name, "_index.md") {
				continue
			}
			if filepath.Ext(name) == ".md" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			return nil
		}
		rel, relErr := filepath.Rel(contentDir, indexPath)
		if relErr != nil {
			return relErr
		}
		content := normalizeMarkdown(nil, filepath.ToSlash(rel), markdownPrepOptions{}, 0, false)
		return os.WriteFile(indexPath, content, 0o644)
	})
}

func normalizeMarkdown(raw []byte, rel string, opts markdownPrepOptions, weight int, hasWeight bool) []byte {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\uFEFF")
	if strings.HasPrefix(text, "---\n") {
		return normalizeYAMLFrontMatter(text, rel, opts, weight, hasWeight)
	}
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
	if hasWeight {
		frontMatter = append(frontMatter, fmt.Sprintf("weight: %d", weight))
	}
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

func normalizeYAMLFrontMatter(text string, rel string, opts markdownPrepOptions, weight int, hasWeight bool) []byte {
	frontMatterText, body, ok := splitYAMLFrontMatter(text)
	if !ok {
		return []byte(text)
	}
	data := map[string]any{}
	if err := yaml.Unmarshal([]byte(frontMatterText), &data); err != nil {
		return []byte(text)
	}
	if data == nil {
		data = map[string]any{}
	}
	if rel == "_index.md" && opts.RootTitle != "" {
		if strings.TrimSpace(fmt.Sprint(data["title"])) == "" || data["title"] == nil {
			data["title"] = opts.RootTitle
		}
	}
	if strings.TrimSpace(fmt.Sprint(data["title"])) == "" || data["title"] == nil {
		bodyTitle, _ := extractLeadingH1(body)
		if bodyTitle != "" {
			data["title"] = bodyTitle
		} else {
			data["title"] = fallbackTitleFromPath(rel)
		}
	}
	if hasWeight {
		data["weight"] = weight
	}
	if rel == "_index.md" && opts.RootCascadeDocs {
		ensureCascadeDocs(data)
	}
	return marshalYAMLFrontMatter(data, body)
}

func splitYAMLFrontMatter(text string) (string, string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			frontMatter := strings.Join(lines[1:i], "\n")
			body := strings.Join(lines[i+1:], "\n")
			return frontMatter, body, true
		}
	}
	return "", "", false
}

func marshalYAMLFrontMatter(data map[string]any, body string) []byte {
	raw, err := yaml.Marshal(data)
	if err != nil {
		return []byte(body)
	}
	result := strings.Builder{}
	result.WriteString("---\n")
	result.Write(raw)
	result.WriteString("---\n")
	if body != "" {
		result.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			result.WriteString("\n")
		}
	}
	return []byte(result.String())
}

func ensureCascadeDocs(data map[string]any) {
	cascade, ok := data["cascade"]
	if !ok || cascade == nil {
		data["cascade"] = map[string]any{"type": "docs"}
		return
	}
	cascadeMap, ok := cascade.(map[string]any)
	if !ok {
		return
	}
	if strings.TrimSpace(fmt.Sprint(cascadeMap["type"])) == "" || cascadeMap["type"] == nil {
		cascadeMap["type"] = "docs"
	}
}

func buildSidebarWeights(files []string, sidebarOrder []string) map[string]int {
	weights := make(map[string]int, len(files))
	groups := make(map[string][]string)
	for _, rel := range files {
		group := sidebarGroupKey(rel)
		groups[group] = append(groups[group], rel)
	}
	configured := make(map[string]int, len(sidebarOrder))
	inferredSections := make(map[string]int)
	for idx, item := range sidebarOrder {
		target := normalizeSidebarOrderItem(item)
		if target == "" {
			continue
		}
		weight := (idx + 1) * 10
		configured[target] = weight
		for _, sectionRel := range ancestorSectionIndexes(target) {
			current, ok := inferredSections[sectionRel]
			if !ok || weight < current {
				inferredSections[sectionRel] = weight
			}
		}
	}
	defaultStart := (len(sidebarOrder) + 1) * 10
	for _, rels := range groups {
		sort.Strings(rels)
		nextWeight := defaultStart
		for _, rel := range rels {
			if configuredWeight, ok := configured[rel]; ok {
				weights[rel] = configuredWeight
				continue
			}
			if inferredWeight, ok := inferredSections[rel]; ok {
				weights[rel] = inferredWeight
			}
		}
		for _, rel := range rels {
			if _, ok := weights[rel]; ok {
				continue
			}
			weights[rel] = nextWeight
			nextWeight += 10
		}
	}
	return weights
}

func ancestorSectionIndexes(rel string) []string {
	parent := filepath.ToSlash(filepath.Dir(rel))
	if parent == "." || parent == "" {
		return nil
	}
	sections := make([]string, 0, 4)
	for parent != "." && parent != "" {
		sections = append(sections, filepath.ToSlash(filepath.Join(parent, "_index.md")))
		parent = filepath.ToSlash(filepath.Dir(parent))
	}
	return sections
}

func normalizeSidebarOrderItem(item string) string {
	cleaned := strings.TrimSpace(item)
	cleaned = strings.Trim(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "" || cleaned == "." {
		return ""
	}
	if strings.HasSuffix(cleaned, "/_index.md") || strings.HasSuffix(cleaned, ".md") {
		return cleaned
	}
	return cleaned + "/_index.md"
}

func sidebarGroupKey(rel string) string {
	cleaned := filepath.ToSlash(rel)
	if cleaned == "_index.md" {
		return "."
	}
	if strings.HasSuffix(cleaned, "/_index.md") {
		sectionPath := strings.TrimSuffix(cleaned, "/_index.md")
		parent := filepath.ToSlash(filepath.Dir(sectionPath))
		if parent == "." || parent == "" {
			return "."
		}
		return parent
	}
	parent := filepath.ToSlash(filepath.Dir(cleaned))
	if parent == "." || parent == "" {
		return "."
	}
	return parent
}

func extractLeadingH1(text string) (string, string) {
	text = strings.TrimPrefix(text, "\uFEFF")
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
	text = strings.TrimPrefix(text, "\uFEFF")
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
