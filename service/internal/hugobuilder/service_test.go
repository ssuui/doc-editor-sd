package hugobuilder

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"doc-publish-server/internal/configloader"
)

func TestNormalizeYAMLFrontMatterUsesBodyH1AsTitle(t *testing.T) {
	input := strings.Join([]string{
		"---",
		"weight: 20",
		"---",
		"",
		"# 页面标题",
		"",
		"正文",
	}, "\n")

	output := string(normalizeMarkdown([]byte(input), "guide.md", markdownPrepOptions{}, 0, false))

	if !strings.Contains(output, "title: 页面标题\n") {
		t.Fatalf("expected body h1 to be promoted into front matter title, got:\n%s", output)
	}
}

func TestNormalizeMarkdownSupportsUTF8BOMBeforeH1(t *testing.T) {
	input := "\uFEFF# 带 BOM 的标题\n\n正文\n"

	output := string(normalizeMarkdown([]byte(input), "guide.md", markdownPrepOptions{}, 0, false))

	if !strings.Contains(output, "title: \"带 BOM 的标题\"") {
		t.Fatalf("expected BOM-prefixed h1 to be extracted as title, got:\n%s", output)
	}
}

func TestBuildSidebarWeightsPromotesSectionsFromConfiguredChildren(t *testing.T) {
	files := []string{
		"_index.md",
		"README.md",
		"quick-start/_index.md",
		"quick-start/install.md",
		"quick-start/overview.md",
		"lifecycle.md",
	}

	weights := buildSidebarWeights(files, []string{
		"README.md",
		"quick-start/install.md",
		"quick-start/overview.md",
		"lifecycle.md",
	})

	if weights["quick-start/_index.md"] != 20 {
		t.Fatalf("expected section to inherit first configured child weight, got %d", weights["quick-start/_index.md"])
	}
	if weights["quick-start/install.md"] != 20 {
		t.Fatalf("expected configured child weight to remain stable, got %d", weights["quick-start/install.md"])
	}
	if weights["lifecycle.md"] != 40 {
		t.Fatalf("expected later root page weight to remain stable, got %d", weights["lifecycle.md"])
	}
}

func TestAncestorSectionIndexes(t *testing.T) {
	got := ancestorSectionIndexes("guide/setup/install.md")
	want := []string{"guide/setup/_index.md", "guide/_index.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ancestor sections: got %v want %v", got, want)
	}
}

func TestEnsureSectionIndexFilesCreatesMissingIndexes(t *testing.T) {
	contentDir := t.TempDir()
	sectionDir := filepath.Join(contentDir, "01-快速开始")
	if err := os.MkdirAll(filepath.Join(sectionDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	pagePath := filepath.Join(sectionDir, "安装.md")
	if err := os.WriteFile(pagePath, []byte("# 安装\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nestedPagePath := filepath.Join(sectionDir, "nested", "更多说明.md")
	if err := os.WriteFile(nestedPagePath, []byte("# 更多说明\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureSectionIndexFiles(contentDir); err != nil {
		t.Fatalf("ensureSectionIndexFiles failed: %v", err)
	}

	indexRaw, err := os.ReadFile(filepath.Join(sectionDir, "_index.md"))
	if err != nil {
		t.Fatalf("expected generated _index.md: %v", err)
	}
	indexText := string(indexRaw)
	if !strings.Contains(indexText, "title: \"快速开始\"") {
		t.Fatalf("expected generated title from directory name, got:\n%s", indexText)
	}
	if _, err := os.Stat(filepath.Join(sectionDir, "nested", "_index.md")); err != nil {
		t.Fatalf("expected nested directory to receive _index.md: %v", err)
	}
}

func TestGenerateBooksPageHidesRenderedTitle(t *testing.T) {
	sourceRoot := t.TempDir()
	booksMeta := []byte("book_list:\n  - book_dir_name: book-a\n    weight: 10\n")
	if err := os.WriteFile(filepath.Join(sourceRoot, "_books_meta.yaml"), booksMeta, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "book-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	bookMeta := []byte("display_name: 示例文档\ndescription: 说明\nversion: v1\n")
	if err := os.WriteFile(filepath.Join(sourceRoot, "book-a", "book_meta.yaml"), bookMeta, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &Service{siteCfg: &configloader.SiteGlobalConfig{}}
	output, err := svc.generateBooksPage(sourceRoot, "全部书籍")
	if err != nil {
		t.Fatalf("generateBooksPage failed: %v", err)
	}

	if !strings.Contains(output, "hideTitle: true") {
		t.Fatalf("expected generated page to hide rendered title, got:\n%s", output)
	}
	if strings.Contains(output, "# 全部书籍") {
		t.Fatalf("expected generated page to avoid leading h1 duplication, got:\n%s", output)
	}
	if !strings.Contains(output, "books-page-search-input") {
		t.Fatalf("expected generated books page to include a search box, got:\n%s", output)
	}
}
