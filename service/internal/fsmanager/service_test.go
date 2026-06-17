package fsmanager

import (
	"os"
	"path/filepath"
	"testing"

	"doc-publish-server/internal/configloader"
)

func TestScanBooksOnlyAcceptsBPrefix(t *testing.T) {
	root := t.TempDir()

	writeBook := func(dir string, displayName string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		raw := []byte("display_name: " + displayName + "\n")
		if err := os.WriteFile(filepath.Join(root, dir, "book_meta.yaml"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeBook("b_02_beta", "Beta")
	writeBook("b_01_alpha", "Alpha")
	writeBook("book_legacy", "Legacy")

	svc := &Service{
		root: root,
		cfg:  &configloader.SystemConfig{},
	}

	books, err := svc.scanBooks()
	if err != nil {
		t.Fatalf("scanBooks failed: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("expected 2 books, got %d", len(books))
	}
	if books[0].dirName != "b_01_alpha" || books[1].dirName != "b_02_beta" {
		t.Fatalf("unexpected scan result order: %+v", books)
	}
}
