package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"doc-publish-server/internal/api"
	"doc-publish-server/internal/auth"
	"doc-publish-server/internal/configloader"
	"doc-publish-server/internal/filelock"
	"doc-publish-server/internal/fsmanager"
	"doc-publish-server/internal/hugobuilder"
	"doc-publish-server/internal/publishtask"
	"doc-publish-server/internal/recordstore"
	"doc-publish-server/internal/s3uploader"

	"github.com/gin-gonic/gin"
)

func main() {
	systemCfg, err := configloader.LoadSystemConfig("./config/system.yaml")
	if err != nil {
		log.Fatal(err)
	}
	siteCfg, err := configloader.LoadSiteGlobalConfig("./config/site_global.yaml")
	if err != nil {
		log.Fatal(err)
	}

	mustEnsureDirs(systemCfg)

	filelock.StartCleanup(30*time.Minute, 2*time.Hour)
	auth.StartCleanup()
	go cleanTemp(systemCfg.BuildTempRoot, time.Duration(systemCfg.TempCleanInterval)*time.Hour)

	hugoSvc := hugobuilder.New(systemCfg, siteCfg)
	if err := hugoSvc.CheckBinary(); err != nil {
		log.Fatal(err)
	}

	uploaderSvc, err := s3uploader.New(systemCfg)
	if err != nil {
		log.Fatal(err)
	}

	router := gin.Default()
	api.Register(router, api.Services{
		System:    systemCfg,
		Site:      siteCfg,
		FS:        fsmanager.New(systemCfg.SourceRootPath, systemCfg),
		Hugo:      hugoSvc,
		Uploader:  uploaderSvc,
		Tasks:     publishtask.New(),
		Records:   recordstore.New(systemCfg.PublishRecordPath),
		SourceDir: systemCfg.SourceRootPath,
	}, "./static_resources")

	log.Fatal(router.Run((fmt.Sprintf(":%d", systemCfg.HTTPPort))))
}

func mustEnsureDirs(cfg *configloader.SystemConfig) {
	for _, dir := range []string{
		cfg.SourceRootPath,
		cfg.BuildTempRoot,
		cfg.PublishRecordPath,
		"./static_resources",
		filepath.Dir(cfg.HugoBinPath),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
	}
}

func cleanTemp(root string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
		_ = os.MkdirAll(filepath.Join(root, "main_site_out"), 0o755)
		_ = os.MkdirAll(filepath.Join(root, "book_cache"), 0o755)
		_ = os.MkdirAll(filepath.Join(root, "full_package"), 0o755)
	}
}
