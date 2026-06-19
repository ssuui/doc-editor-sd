package indexmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"doc-publish-server/internal/configloader"
)

type Service struct {
	root string
}

type PlanRequest struct {
	Targets       []string `json:"targets"`
	Mode          string   `json:"mode"`
	Preview       bool     `json:"preview"`
	RemoveMissing bool     `json:"remove_missing"`
}

type ChangeItem struct {
	Type        string `json:"type" yaml:"type"`
	BookDirName string `json:"book_dir_name" yaml:"book_dir_name"`
	Detail      string `json:"detail" yaml:"detail"`
}

type PlanResult struct {
	Targets []string                              `json:"targets"`
	Mode    string                                `json:"mode"`
	Site    *MetaPlan[configloader.SiteBookItem]  `json:"site,omitempty"`
	Books   *MetaPlan[configloader.BooksBookItem] `json:"books,omitempty"`
}

type MetaPlan[T any] struct {
	Changed bool         `json:"changed"`
	Items   []T          `json:"items"`
	Changes []ChangeItem `json:"changes"`
}

func New(root string) *Service {
	return &Service{root: filepath.Clean(root)}
}

func (s *Service) Plan(req PlanRequest) (*PlanResult, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "append_new"
	}
	books, err := s.scanBooks()
	if err != nil {
		return nil, err
	}
	targets := normalizeTargets(req.Targets)
	result := &PlanResult{Targets: targets, Mode: mode}
	for _, target := range targets {
		switch target {
		case "site_meta":
			plan, err := s.planSite(mode, req.RemoveMissing, books)
			if err != nil {
				return nil, err
			}
			result.Site = plan
		case "books_meta":
			plan, err := s.planBooks(mode, req.RemoveMissing, books)
			if err != nil {
				return nil, err
			}
			result.Books = plan
		default:
			return nil, fmt.Errorf("未知索引目标: %s", target)
		}
	}
	return result, nil
}

func (s *Service) Apply(req PlanRequest) (*PlanResult, error) {
	plan, err := s.Plan(req)
	if err != nil {
		return nil, err
	}
	if plan.Site != nil {
		if err := configloader.SaveYAML(filepath.Join(s.root, "_site_meta.yaml"), &configloader.SiteMeta{BookList: plan.Site.Items}); err != nil {
			return nil, err
		}
	}
	if plan.Books != nil {
		if err := configloader.SaveYAML(filepath.Join(s.root, "_books_meta.yaml"), &configloader.BooksMeta{BookList: plan.Books.Items}); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func normalizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return []string{"site_meta", "books_meta"}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	return result
}

type scannedBook struct {
	dirName       string
	visibleInHome bool
}

func (s *Service) scanBooks() ([]scannedBook, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	books := make([]scannedBook, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "b_") {
			continue
		}
		metaPath := filepath.Join(s.root, entry.Name(), "book_meta.yaml")
		if _, err := os.Stat(metaPath); err != nil {
			continue
		}
		meta, err := configloader.LoadBookMeta(metaPath)
		if err != nil {
			continue
		}
		books = append(books, scannedBook{
			dirName:       entry.Name(),
			visibleInHome: meta.VisibleInHome,
		})
	}
	sort.Slice(books, func(i, j int) bool { return books[i].dirName < books[j].dirName })
	return books, nil
}

func (s *Service) planSite(mode string, removeMissing bool, scanned []scannedBook) (*MetaPlan[configloader.SiteBookItem], error) {
	current, err := configloader.LoadSiteMeta(filepath.Join(s.root, "_site_meta.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	items, changes := planSiteItems(mode, removeMissing, current, scanned)
	return &MetaPlan[configloader.SiteBookItem]{Changed: len(changes) > 0, Items: items, Changes: changes}, nil
}

func (s *Service) planBooks(mode string, removeMissing bool, scanned []scannedBook) (*MetaPlan[configloader.BooksBookItem], error) {
	current, err := configloader.LoadBooksMeta(filepath.Join(s.root, "_books_meta.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	items, changes := planBooksItems(mode, removeMissing, current, scanned)
	return &MetaPlan[configloader.BooksBookItem]{Changed: len(changes) > 0, Items: items, Changes: changes}, nil
}

func planSiteItems(mode string, removeMissing bool, current *configloader.SiteMeta, scanned []scannedBook) ([]configloader.SiteBookItem, []ChangeItem) {
	currentItems := []configloader.SiteBookItem{}
	if current != nil {
		currentItems = append(currentItems, current.BookList...)
	}
	return planMetaItems(mode, removeMissing, currentItems, scanned, metaItemOps[configloader.SiteBookItem]{
		dirName: func(item configloader.SiteBookItem) string { return item.BookDirName },
		weight:  func(item configloader.SiteBookItem) int { return item.Weight },
		build: func(book scannedBook, weight int) configloader.SiteBookItem {
			return configloader.SiteBookItem{
				BookDirName:    book.dirName,
				Weight:         weight,
				EnableHomeShow: book.visibleInHome,
			}
		},
	})
}

func planBooksItems(mode string, removeMissing bool, current *configloader.BooksMeta, scanned []scannedBook) ([]configloader.BooksBookItem, []ChangeItem) {
	currentItems := []configloader.BooksBookItem{}
	if current != nil {
		currentItems = append(currentItems, current.BookList...)
	}
	return planMetaItems(mode, removeMissing, currentItems, scanned, metaItemOps[configloader.BooksBookItem]{
		dirName: func(item configloader.BooksBookItem) string { return item.BookDirName },
		weight:  func(item configloader.BooksBookItem) int { return item.Weight },
		build: func(book scannedBook, weight int) configloader.BooksBookItem {
			return configloader.BooksBookItem{BookDirName: book.dirName, Weight: weight}
		},
	})
}

type metaItemOps[T any] struct {
	dirName func(T) string
	weight  func(T) int
	build   func(scannedBook, int) T
}

func planMetaItems[T any](mode string, removeMissing bool, currentItems []T, scanned []scannedBook, ops metaItemOps[T]) ([]T, []ChangeItem) {
	scannedMap := map[string]scannedBook{}
	for _, book := range scanned {
		scannedMap[book.dirName] = book
	}
	if mode == "full_refresh" {
		items := make([]T, 0, len(scanned))
		changes := make([]ChangeItem, 0, len(scanned))
		weight := 10
		for _, book := range scanned {
			items = append(items, ops.build(book, weight))
			changes = append(changes, ChangeItem{Type: "reset", BookDirName: book.dirName, Detail: fmt.Sprintf("重置为权重 %d", weight)})
			weight += 10
		}
		return items, changes
	}
	items := append([]T(nil), currentItems...)
	changes := []ChangeItem{}
	maxWeight := maxMetaWeight(items, ops.weight)
	exists := map[string]bool{}
	for _, item := range items {
		exists[ops.dirName(item)] = true
	}
	for _, book := range scanned {
		if exists[book.dirName] {
			continue
		}
		maxWeight += 10
		items = append(items, ops.build(book, maxWeight))
		changes = append(changes, ChangeItem{Type: "append", BookDirName: book.dirName, Detail: fmt.Sprintf("新增到末尾，权重 %d", maxWeight)})
	}
	if removeMissing {
		filtered := items[:0]
		for _, item := range items {
			if _, ok := scannedMap[ops.dirName(item)]; ok {
				filtered = append(filtered, item)
				continue
			}
			changes = append(changes, ChangeItem{Type: "remove_missing", BookDirName: ops.dirName(item), Detail: "目录不存在，已清理"})
		}
		items = filtered
	}
	return items, changes
}

func maxMetaWeight[T any](items []T, getter func(T) int) int {
	maxWeight := 0
	for _, item := range items {
		if weight := getter(item); weight > maxWeight {
			maxWeight = weight
		}
	}
	return maxWeight
}
