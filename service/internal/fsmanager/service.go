package fsmanager

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"doc-publish-server/internal/configloader"
	"doc-publish-server/internal/filelock"
)

type Service struct {
	root string
	cfg  *configloader.SystemConfig
}

type Node struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Children []Node `json:"children,omitempty"`
}

func New(root string, cfg *configloader.SystemConfig) *Service {
	return &Service{root: filepath.Clean(root), cfg: cfg}
}

func (s *Service) SiteTree() ([]Node, error) {
	return s.scanTree(".")
}

func (s *Service) BookTree(bookDir string) ([]Node, error) {
	return s.scanTree(bookDir)
}

func (s *Service) ReadFile(rel string) (string, error) {
	abs, err := s.safePath(rel)
	if err != nil {
		return "", err
	}
	filelock.RLockFile(abs)
	defer filelock.RUnlockFile(abs)
	raw, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Service) ResolvePath(rel string) (string, error) {
	return s.safePath(rel)
}

func (s *Service) SaveFile(rel string, content string) error {
	abs, err := s.safePath(rel)
	if err != nil {
		return err
	}
	filelock.LockFile(abs)
	defer filelock.UnlockFile(abs)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

func (s *Service) NewPath(kind string, rel string) error {
	abs, err := s.safePath(rel)
	if err != nil {
		return err
	}
	filelock.LockFile(abs)
	defer filelock.UnlockFile(abs)
	if kind == "folder" {
		return os.MkdirAll(abs, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(""), 0o644)
}

func (s *Service) RemovePath(rel string) error {
	abs, err := s.safePath(rel)
	if err != nil {
		return err
	}
	filelock.LockFile(abs)
	defer filelock.UnlockFile(abs)
	return os.RemoveAll(abs)
}

func (s *Service) RenamePath(rel string, newName string) (string, error) {
	abs, err := s.safePath(rel)
	if err != nil {
		return "", err
	}
	if strings.Contains(newName, "..") || newName == "" {
		return "", errIllegalPath
	}
	var target string
	if strings.Contains(newName, "/") {
		target, err = s.safePath(newName)
		if err != nil {
			return "", err
		}
	} else {
		target = filepath.Join(filepath.Dir(abs), newName)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	filelock.LockFile(abs)
	defer filelock.UnlockFile(abs)
	target, err = s.uniqueTargetPath(target)
	if err != nil {
		return "", err
	}
	if err := os.Rename(abs, target); err != nil {
		return "", err
	}
	return s.relPath(target), nil
}

func (s *Service) CopyPath(rel string) (string, error) {
	source, err := s.safePath(rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	target, err := s.uniqueTargetPath(s.duplicateCandidatePath(source))
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if err := copyDir(source, target); err != nil {
			return "", err
		}
	} else {
		if err := copyFile(source, target); err != nil {
			return "", err
		}
	}
	return s.relPath(target), nil
}

// UploadFile 将上传的文件写入源码目录。同名文件自动改名为"xxx-副本.ext"。
// targetDir 是相对于 source_root 的目录路径(如 b_01/chapter)。
func (s *Service) UploadFile(targetDir string, filename string, data []byte) (string, error) {
	absDir, err := s.safePath(targetDir)
	if err != nil {
		return "", err
	}
	abs, err := s.safePath(filepath.Join(targetDir, filename))
	if err != nil {
		return "", err
	}
	// 同名时自动改名为副本
	abs, err = s.uniqueTargetPath(abs)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	return s.relPath(abs), nil
}

func (s *Service) PresignUpload(bookDir string, ext string) (string, string, string) {
	storeDir := "static-asset"
	if isImageExt(ext) {
		storeDir = "static-img"
	}
	token := fmt.Sprintf("%d_%s%s", time.Now().Unix(), randomID(), ext)
	key := strings.TrimLeft(filepath.ToSlash(filepath.Join(s.cfg.S3.ImgStorePrefix, bookDir, storeDir, token)), "/")
	putURL := fmt.Sprintf("%s/%s?presigned=mock", strings.TrimRight(s.cfg.S3.Endpoint, "/"), key)
	cdnURL := fmt.Sprintf("https://%s/%s", strings.TrimRight(s.cfg.S3.ImgCDNDomain, "/"), key)
	return key, putURL, cdnURL
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return true
	}
	return false
}

func (s *Service) safePath(rel string) (string, error) {
	clean := filepath.Clean("/" + rel)
	if strings.Contains(clean, "..") {
		return "", errIllegalPath
	}
	abs := filepath.Join(s.root, clean)
	if !strings.HasPrefix(filepath.Clean(abs), s.root) {
		return "", errIllegalPath
	}
	return abs, nil
}

func (s *Service) scanTree(rel string) ([]Node, error) {
	abs, err := s.safePath(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(entries))
	for _, entry := range entries {
		itemRel := filepath.ToSlash(filepath.Join(strings.TrimPrefix(rel, "./"), entry.Name()))
		node := Node{Name: entry.Name(), Path: itemRel}
		if entry.IsDir() {
			node.Type = "folder"
			children, err := s.scanTree(itemRel)
			if err == nil {
				node.Children = children
			}
		} else {
			node.Type = "file"
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type == nodes[j].Type {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Type == "folder"
	})
	return nodes, nil
}

func ListBooks(metaPath string) ([]configloader.BooksBookItem, error) {
	meta, err := configloader.LoadBooksMeta(metaPath)
	if err != nil {
		return nil, err
	}
	sort.Slice(meta.BookList, func(i, j int) bool { return meta.BookList[i].Weight < meta.BookList[j].Weight })
	return meta.BookList, nil
}

func (s *Service) RebuildSiteMeta() ([]configloader.SiteBookItem, error) {
	books, err := s.scanBooks()
	if err != nil {
		return nil, err
	}
	items := make([]configloader.SiteBookItem, 0, len(books))
	weight := 10
	for _, book := range books {
		items = append(items, configloader.SiteBookItem{
			BookDirName:    book.dirName,
			Weight:         weight,
			EnableHomeShow: book.meta.VisibleInHome,
		})
		weight += 10
	}
	meta := &configloader.SiteMeta{BookList: items}
	if err := configloader.SaveYAML(filepath.Join(s.root, "_site_meta.yaml"), meta); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) RebuildBooksMeta() ([]configloader.BooksBookItem, error) {
	books, err := s.scanBooks()
	if err != nil {
		return nil, err
	}
	items := make([]configloader.BooksBookItem, 0, len(books))
	weight := 10
	for _, book := range books {
		items = append(items, configloader.BooksBookItem{
			BookDirName: book.dirName,
			Weight:      weight,
		})
		weight += 10
	}
	meta := &configloader.BooksMeta{BookList: items}
	if err := configloader.SaveYAML(filepath.Join(s.root, "_books_meta.yaml"), meta); err != nil {
		return nil, err
	}
	return items, nil
}

func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

var errIllegalPath = errors.New("illegal path")

func IsIllegalPath(err error) bool {
	return errors.Is(err, errIllegalPath)
}

func randomID() string {
	return strings.ToLower(strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()), "-", ""))
}

func (s *Service) relPath(abs string) string {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return filepath.ToSlash(filepath.Base(abs))
	}
	return filepath.ToSlash(rel)
}

func (s *Service) uniqueTargetPath(target string) (string, error) {
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		return target, nil
	} else if err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	name := filepath.Base(target)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for index := 1; ; index++ {
		candidateName := duplicateName(base, ext, index)
		candidate := filepath.Join(dir, candidateName)
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (s *Service) duplicateCandidatePath(source string) string {
	dir := filepath.Dir(source)
	name := filepath.Base(source)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return filepath.Join(dir, duplicateName(base, ext, 1))
}

func duplicateName(base string, ext string, index int) string {
	if index <= 1 {
		return base + "-副本" + ext
	}
	return fmt.Sprintf("%s-副本%d%s", base, index, ext)
}

func copyFile(source string, target string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, raw, 0o644)
}

func copyDir(source string, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		return copyFile(path, dest)
	})
}

type scannedBook struct {
	dirName string
	meta    *configloader.BookMeta
}

func (s *Service) scanBooks() ([]scannedBook, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	books := make([]scannedBook, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "b_") {
			continue
		}
		bookMetaPath := filepath.Join(s.root, name, "book_meta.yaml")
		if _, err := os.Stat(bookMetaPath); err != nil {
			continue
		}
		meta, err := configloader.LoadBookMeta(bookMetaPath)
		if err != nil {
			continue
		}
		books = append(books, scannedBook{
			dirName: name,
			meta:    meta,
		})
	}
	sort.Slice(books, func(i, j int) bool {
		return books[i].dirName < books[j].dirName
	})
	return books, nil
}
