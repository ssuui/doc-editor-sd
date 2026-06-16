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

func (s *Service) RenamePath(rel string, newName string) error {
	abs, err := s.safePath(rel)
	if err != nil {
		return err
	}
	if strings.Contains(newName, "/") || strings.Contains(newName, "..") || newName == "" {
		return errIllegalPath
	}
	target := filepath.Join(filepath.Dir(abs), newName)
	filelock.LockFile(abs)
	defer filelock.UnlockFile(abs)
	return os.Rename(abs, target)
}

func (s *Service) PresignUpload(bookDir string, ext string) (string, string, string) {
	token := fmt.Sprintf("%d_%s%s", time.Now().Unix(), randomID(), ext)
	key := strings.TrimLeft(filepath.ToSlash(filepath.Join(s.cfg.S3.ImgStorePrefix, bookDir, "static-img", token)), "/")
	putURL := fmt.Sprintf("%s/%s?presigned=mock", strings.TrimRight(s.cfg.S3.Endpoint, "/"), key)
	cdnURL := fmt.Sprintf("https://%s/%s", strings.TrimRight(s.cfg.S3.ImgCDNDomain, "/"), key)
	return key, putURL, cdnURL
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

func ListBooks(metaPath string) ([]configloader.SiteBookItem, error) {
	meta, err := configloader.LoadSiteMeta(metaPath)
	if err != nil {
		return nil, err
	}
	sort.Slice(meta.BookList, func(i, j int) bool { return meta.BookList[i].Weight < meta.BookList[j].Weight })
	return meta.BookList, nil
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
