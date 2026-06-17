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

func TestBuildSidebarWeightsAlignsLeafPagesWithSectionOrder(t *testing.T) {
	files := []string{
		"_index.md",
		"02-review/_index.md",
		"02-review/checklist.md",
		"01-setup/_index.md",
		"01-setup/start.md",
		"03-flow/_index.md",
		"03-flow/commit.md",
	}

	weights := buildSidebarWeights(files, []string{
		"02-review",
		"01-setup",
	})

	if weights["02-review/checklist.md"] >= weights["01-setup/start.md"] {
		t.Fatalf("expected first configured section child to sort before second section child, got %d >= %d", weights["02-review/checklist.md"], weights["01-setup/start.md"])
	}
	if weights["01-setup/start.md"] >= weights["03-flow/commit.md"] {
		t.Fatalf("expected configured section child to sort before default section child, got %d >= %d", weights["01-setup/start.md"], weights["03-flow/commit.md"])
	}
}

func TestPrepareMarkdownTreeAssignsLeafWeightsFromSidebarOrder(t *testing.T) {
	contentDir := t.TempDir()
	files := map[string]string{
		"_index.md":                    "---\ntitle: 根\n---\n",
		"02-review/_index.md":         "---\ntitle: 评审\n---\n",
		"02-review/checklist.md":      "# 评审清单\n",
		"01-setup/_index.md":          "---\ntitle: 环境\n---\n",
		"01-setup/start.md":           "# 本地启动\n",
		"03-flow/_index.md":           "---\ntitle: 流程\n---\n",
		"03-flow/commit.md":           "# 提交流程\n",
	}
	for rel, content := range files {
		path := filepath.Join(contentDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	svc := &Service{}
	if err := svc.prepareMarkdownTree(contentDir, markdownPrepOptions{
		RootTitle:       "示例书本",
		RootCascadeDocs: true,
		ForceDocsType:   true,
		SidebarOrder:    []string{"02-review", "01-setup"},
	}); err != nil {
		t.Fatalf("prepareMarkdownTree failed: %v", err)
	}

	checklistRaw, err := os.ReadFile(filepath.Join(contentDir, "02-review", "checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	startRaw, err := os.ReadFile(filepath.Join(contentDir, "01-setup", "start.md"))
	if err != nil {
		t.Fatal(err)
	}
	commitRaw, err := os.ReadFile(filepath.Join(contentDir, "03-flow", "commit.md"))
	if err != nil {
		t.Fatal(err)
	}

	checklistText := string(checklistRaw)
	startText := string(startRaw)
	commitText := string(commitRaw)
	if !strings.Contains(checklistText, "weight: 11") {
		t.Fatalf("expected first configured section leaf to inherit early weight, got:\n%s", checklistText)
	}
	if !strings.Contains(startText, "weight: 21") {
		t.Fatalf("expected second configured section leaf to follow next, got:\n%s", startText)
	}
	if !strings.Contains(commitText, "weight: 30") {
		t.Fatalf("expected default section leaf to follow configured sections, got:\n%s", commitText)
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
	booksMeta := []byte("book_list:\n  - book_dir_name: b-a\n    weight: 10\n")
	if err := os.WriteFile(filepath.Join(sourceRoot, "_books_meta.yaml"), booksMeta, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "b-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	bookMeta := []byte("display_name: 示例文档\ndescription: 说明\nversion: v1\n")
	if err := os.WriteFile(filepath.Join(sourceRoot, "b-a", "book_meta.yaml"), bookMeta, 0o644); err != nil {
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
