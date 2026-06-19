package indexmanager

import (
	"testing"

	"doc-publish-server/internal/configloader"
)

func TestPlanSiteItemsAppendNewKeepsExistingOrder(t *testing.T) {
	current := &configloader.SiteMeta{
		BookList: []configloader.SiteBookItem{
			{BookDirName: "b_002", Weight: 20, EnableHomeShow: false},
		},
	}
	scanned := []scannedBook{
		{dirName: "b_001", visibleInHome: true},
		{dirName: "b_002", visibleInHome: true},
	}

	items, changes := planSiteItems("append_new", false, current, scanned)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].BookDirName != "b_002" || items[0].Weight != 20 {
		t.Fatalf("expected existing item to stay first, got %+v", items[0])
	}
	if items[1].BookDirName != "b_001" || items[1].Weight != 30 || !items[1].EnableHomeShow {
		t.Fatalf("expected appended item with inherited home visibility, got %+v", items[1])
	}
	if len(changes) != 1 || changes[0].Type != "append" || changes[0].BookDirName != "b_001" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}

func TestPlanBooksItemsFullRefreshResetsWeights(t *testing.T) {
	scanned := []scannedBook{
		{dirName: "b_010"},
		{dirName: "b_020"},
	}

	items, changes := planBooksItems("full_refresh", false, nil, scanned)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].BookDirName != "b_010" || items[0].Weight != 10 {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].BookDirName != "b_020" || items[1].Weight != 20 {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
	if len(changes) != 2 || changes[0].Type != "reset" || changes[1].Type != "reset" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}

func TestPlanBooksItemsRemoveMissing(t *testing.T) {
	current := &configloader.BooksMeta{
		BookList: []configloader.BooksBookItem{
			{BookDirName: "b_001", Weight: 10},
			{BookDirName: "b_999", Weight: 20},
		},
	}
	scanned := []scannedBook{{dirName: "b_001"}}

	items, changes := planBooksItems("append_new", true, current, scanned)

	if len(items) != 1 || items[0].BookDirName != "b_001" {
		t.Fatalf("expected only existing scanned item to remain, got %+v", items)
	}
	if len(changes) != 1 || changes[0].Type != "remove_missing" || changes[0].BookDirName != "b_999" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}
