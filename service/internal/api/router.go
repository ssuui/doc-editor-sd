package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"doc-publish-server/internal/auth"
	"doc-publish-server/internal/configloader"
	"doc-publish-server/internal/fsmanager"
	"doc-publish-server/internal/hugobuilder"
	"doc-publish-server/internal/indexmanager"
	"doc-publish-server/internal/publisher"
	"doc-publish-server/internal/publishtarget"
	"doc-publish-server/internal/publishtask"
	"doc-publish-server/internal/recordstore"
	"doc-publish-server/internal/s3uploader"

	"github.com/gin-gonic/gin"
)

type Services struct {
	System    *configloader.SystemConfig
	Site      *configloader.SiteGlobalConfig
	FS        *fsmanager.Service
	Hugo      *hugobuilder.Service
	Uploader  *s3uploader.Service
	Tasks     *publishtask.Service
	Records   *recordstore.Service
	Indexes   *indexmanager.Service
	Targets   *publishtarget.Service
	SourceDir string
}

func Register(r *gin.Engine, svc Services, staticDir string) {
	r.Use(auth.APIMiddleware())
	r.POST("/api/login", loginHandler(svc.System))
	r.POST("/api/logout", func(c *gin.Context) {
		auth.ClearSession()
		Success(c, gin.H{"ok": true})
	})
	r.GET("/api/build/check-hugo", func(c *gin.Context) {
		if err := svc.Hugo.CheckBinary(); err != nil {
			Fail(c, 3001, err.Error())
			return
		}
		Success(c, gin.H{"ok": true})
	})

	fsGroup := r.Group("/api/fs")
	{
		fsGroup.GET("/site/root-tree", func(c *gin.Context) {
			tree, err := svc.FS.SiteTree()
			respondFileResult(c, tree, err)
		})
		fsGroup.GET("/book/list", func(c *gin.Context) {
			books, err := fsmanager.ListBooks(filepath.Join(svc.SourceDir, "_books_meta.yaml"))
			respondFileResult(c, books, err)
		})
		fsGroup.POST("/site/rebuild-meta", func(c *gin.Context) {
			plan, err := svc.Indexes.Apply(indexmanager.PlanRequest{
				Targets: []string{"site_meta"},
				Mode:    "full_refresh",
			})
			respondFileResult(c, plan.Site.Items, err)
		})
		fsGroup.POST("/site/rebuild-books-meta", func(c *gin.Context) {
			plan, err := svc.Indexes.Apply(indexmanager.PlanRequest{
				Targets: []string{"books_meta"},
				Mode:    "full_refresh",
			})
			respondFileResult(c, plan.Books.Items, err)
		})
		fsGroup.GET("/book/tree", func(c *gin.Context) {
			tree, err := svc.FS.BookTree(c.Query("bookDirName"))
			respondFileResult(c, tree, err)
		})
		fsGroup.GET("/file/content", func(c *gin.Context) {
			content, err := svc.FS.ReadFile(c.Query("path"))
			respondFileResult(c, gin.H{"content": content}, err)
		})
		fsGroup.GET("/file/raw", func(c *gin.Context) {
			abs, err := svc.FS.ResolvePath(c.Query("path"))
			if err != nil {
				respondFileResult(c, nil, err)
				return
			}
			c.File(abs)
		})
		fsGroup.PUT("/file/save", func(c *gin.Context) {
			var req struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			respondFileOnly(c, svc.FS.SaveFile(req.Path, req.Content))
		})
		fsGroup.POST("/file/new", func(c *gin.Context) {
			var req struct {
				Type string `json:"type"`
				Path string `json:"path"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			respondFileOnly(c, svc.FS.NewPath(req.Type, req.Path))
		})
		fsGroup.POST("/file/upload", func(c *gin.Context) {
			targetDir := c.PostForm("target_dir")
			if targetDir == "" || strings.Contains(targetDir, "..") {
				Fail(c, 2003, "非法路径")
				return
			}
			file, header, err := c.Request.FormFile("file")
			if err != nil {
				Fail(c, 9999, "未找到上传文件")
				return
			}
			defer file.Close()
			maxBytes := int64(svc.System.EditorLimit.MaxFileMB) * 1024 * 1024
			data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
			if err != nil {
				Fail(c, 9999, "读取上传文件失败")
				return
			}
			if int64(len(data)) > maxBytes {
				Fail(c, 9999, fmt.Sprintf("文件大小超过 %dMB", svc.System.EditorLimit.MaxFileMB))
				return
			}
			finalPath, err := svc.FS.UploadFile(targetDir, header.Filename, data)
			if err != nil {
				Fail(c, 2002, err.Error())
				return
			}
			respondFileResult(c, gin.H{"path": finalPath}, nil)
		})
		fsGroup.DELETE("/file/remove", func(c *gin.Context) {
			var req struct {
				Path string `json:"path"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			respondFileOnly(c, svc.FS.RemovePath(req.Path))
		})
		fsGroup.POST("/file/copy", func(c *gin.Context) {
			var req struct {
				Path string `json:"path"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			path, err := svc.FS.CopyPath(req.Path)
			respondFileResult(c, gin.H{"path": path}, err)
		})
		fsGroup.PATCH("/file/rename", func(c *gin.Context) {
			var req struct {
				Path    string `json:"path"`
				NewName string `json:"newName"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			path, err := svc.FS.RenamePath(req.Path, req.NewName)
			respondFileResult(c, gin.H{"path": path}, err)
		})
		fsGroup.GET("/get-s3-upload-params", func(c *gin.Context) {
			bookDir := c.Query("bookDirName")
			ext := strings.ToLower(c.DefaultQuery("ext", ".png"))
			if strings.Contains(bookDir, "..") || strings.Contains(bookDir, "/") {
				Fail(c, 2003, "非法路径")
				return
			}
			putURL, cdnURL, contentType, acl, err := svc.Uploader.PresignImageUpload(bookDir, ext)
			if err != nil {
				Fail(c, 4001, err.Error())
				return
			}
			Success(c, gin.H{
				"put_url":      putURL,
				"cdn_img_url":  cdnURL,
				"content_type": contentType,
				"acl":          acl,
			})
		})
	}

	indexGroup := r.Group("/api/index")
	{
		indexGroup.POST("/plan", func(c *gin.Context) {
			var req indexmanager.PlanRequest
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			plan, err := svc.Indexes.Plan(req)
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, plan)
		})
		indexGroup.POST("/apply", func(c *gin.Context) {
			var req indexmanager.PlanRequest
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			plan, err := svc.Indexes.Apply(req)
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, plan)
		})
	}

	pubGroup := r.Group("/api/publish")
	{
		pubGroup.POST("/full-site", func(c *gin.Context) {
			startPublish(c, svc, publishStartRequest{
				Scope:      "full_site",
				Mode:       "incremental",
				TargetType: "s3",
				TargetID:   "system-default",
			})
		})
		pubGroup.POST("/single-book", func(c *gin.Context) {
			book := c.Query("bookDirName")
			startPublish(c, svc, publishStartRequest{
				Scope:      "single_book",
				Books:      []string{book},
				Mode:       "incremental",
				TargetType: "s3",
				TargetID:   "system-default",
			})
		})
		pubGroup.POST("/start", func(c *gin.Context) {
			var req publishStartRequest
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			startPublish(c, svc, req)
		})
		pubGroup.GET("/target/list", func(c *gin.Context) {
			items, err := svc.Targets.List()
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, items)
		})
		pubGroup.GET("/target/detail", func(c *gin.Context) {
			item, err := svc.Targets.Detail(c.Query("type"), c.Query("id"))
			if err != nil {
				Fail(c, 2001, "发布配置不存在")
				return
			}
			Success(c, item)
		})
		pubGroup.POST("/target/save", func(c *gin.Context) {
			var req configloader.PublishTargetConfig
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			if err := svc.Targets.Save(&req); err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, req)
		})
		pubGroup.DELETE("/target/remove", func(c *gin.Context) {
			if err := svc.Targets.Remove(c.Query("type"), c.Query("id")); err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, gin.H{"ok": true})
		})
		pubGroup.POST("/target/test", func(c *gin.Context) {
			var req configloader.PublishTargetConfig
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			pub, err := publisher.New(&req)
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			if err := pub.TestConnection(); err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, gin.H{"ok": true})
		})
		pubGroup.GET("/task/status", func(c *gin.Context) {
			task, ok := svc.Tasks.Get(c.Query("taskId"))
			if !ok {
				Fail(c, 2001, "发布任务不存在")
				return
			}
			Success(c, task)
		})
		pubGroup.GET("/task/stream", func(c *gin.Context) {
			taskID := c.Query("taskId")
			stream, snapshot, ok := svc.Tasks.Subscribe(taskID)
			if !ok {
				Fail(c, 2001, "发布任务不存在")
				return
			}
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			c.Writer.Header().Set("Cache-Control", "no-cache")
			c.Writer.Header().Set("Connection", "keep-alive")
			c.Writer.WriteHeader(http.StatusOK)
			for _, line := range snapshot.Logs {
				writeSSE(c, "log", gin.H{"line": line})
			}
			writeSSE(c, "status", snapshot)
			c.Writer.Flush()
			if snapshot.Done {
				return
			}
			notify := c.Request.Context().Done()
			for {
				select {
				case <-notify:
					return
				case line, ok := <-stream:
					if !ok {
						if task, exists := svc.Tasks.Get(taskID); exists {
							writeSSE(c, "status", task)
							c.Writer.Flush()
						}
						return
					}
					writeSSE(c, "log", gin.H{"line": line})
					c.Writer.Flush()
				}
			}
		})
		pubGroup.GET("/record/list", func(c *gin.Context) {
			records, err := svc.Records.List()
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, records)
		})
		pubGroup.GET("/record/detail", func(c *gin.Context) {
			record, err := svc.Records.Detail(c.Query("recordId"))
			if err != nil {
				Fail(c, 2001, "发布记录不存在")
				return
			}
			Success(c, record)
		})
		pubGroup.GET("/record/files", func(c *gin.Context) {
			record, err := svc.Records.Detail(c.Query("recordId"))
			if err != nil {
				Fail(c, 2001, "发布记录不存在")
				return
			}
			Success(c, record.PublishedFiles)
		})
		pubGroup.POST("/record/restore", func(c *gin.Context) {
			var req struct {
				RecordID string `json:"record_id"`
			}
			if c.ShouldBindJSON(&req) != nil {
				Fail(c, 9999, "请求参数错误")
				return
			}
			record, err := svc.Records.Detail(req.RecordID)
			if err != nil {
				Fail(c, 2001, "发布记录不存在")
				return
			}
			targetCfg, err := svc.Targets.Detail(record.PublishingTargetType, record.PublishingTargetID)
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			pub, err := publisher.New(targetCfg)
			if err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			if err := pub.RestoreBackup(record); err != nil {
				Fail(c, 9999, err.Error())
				return
			}
			Success(c, gin.H{"ok": true})
		})
	}

	r.GET("/", func(c *gin.Context) {
		if !auth.HasValidSession() {
			c.Redirect(http.StatusFound, "/login.html")
			return
		}
		renderConfiguredPage(staticDir, "index.html", svc.Site)(c)
	})
	r.GET("/login.html", renderConfiguredPage(staticDir, "login.html", svc.Site))
	r.GET("/index.html", renderConfiguredPage(staticDir, "index.html", svc.Site))
	r.GET("/styles.css", func(c *gin.Context) { c.File(filepath.Join(staticDir, "styles.css")) })
	r.Static("/assets", filepath.Join(staticDir, "assets"))
}

func renderConfiguredPage(staticDir string, page string, siteCfg *configloader.SiteGlobalConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := filepath.Join(staticDir, page)
		raw, err := os.ReadFile(path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		content := string(raw)
		content = strings.ReplaceAll(content, "__SITE_TITLE__", siteCfg.EffectiveSiteTitle())
		content = strings.ReplaceAll(content, "__ADMIN_TITLE__", siteCfg.AdminTitle())
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
	}
}

func loginHandler(cfg *configloader.SystemConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if c.ShouldBindJSON(&req) != nil {
			Fail(c, 9999, "请求参数错误")
			return
		}
		if req.Username != cfg.Auth.AdminUsername || req.Password != cfg.Auth.AdminPassword {
			Fail(c, 1002, "账号或密码错误")
			return
		}
		token := auth.GenerateToken()
		auth.SetNewSession(token, auth.CalcExpireTs(cfg.Auth.TokenExpireHours))
		Success(c, gin.H{"token": token})
	}
}

type publishStartRequest struct {
	Scope      string   `json:"scope"`
	Books      []string `json:"books"`
	Mode       string   `json:"mode"`
	TargetType string   `json:"target_type"`
	TargetID   string   `json:"target_id"`
}

func startPublish(c *gin.Context, svc Services, req publishStartRequest) {
	scope, books, err := resolvePublishScope(svc, req)
	if err != nil {
		Fail(c, 9999, err.Error())
		return
	}
	task := svc.Tasks.Create(scope, books)
	go runPublishTask(task.ID, svc, req, scope, books)
	Success(c, gin.H{
		"task_id":     task.ID,
		"status":      task.Status,
		"build_books": books,
		"scope":       scope,
	})
}

func resolvePublishScope(svc Services, req publishStartRequest) (string, []string, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "full_site"
	}
	switch scope {
	case "full_site":
		list, err := fsmanager.ListBooks(filepath.Join(svc.SourceDir, "_books_meta.yaml"))
		if err != nil {
			return "", nil, err
		}
		books := make([]string, 0, len(list))
		for _, item := range list {
			books = append(books, item.BookDirName)
		}
		return scope, normalizePublishBooks(books), nil
	case "single_book":
		books := normalizePublishBooks(req.Books)
		if len(books) != 1 {
			return "", nil, fmt.Errorf("请选择一个有效的书籍目录后再发布")
		}
		return scope, books, nil
	case "portal_only":
		return scope, nil, nil
	default:
		return "", nil, fmt.Errorf("未知发布范围: %s", scope)
	}
}

func runPublishTask(taskID string, svc Services, req publishStartRequest, scope string, books []string) {
	logs := []string{}
	appendLog := func(line string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		logs = append(logs, line)
		svc.Tasks.AppendLog(taskID, line)
	}

	targetCfg, err := resolvePublishTarget(svc, req.TargetType, req.TargetID)
	if err != nil {
		failPublishTask(taskID, svc, req, scope, books, "", nil, &logs, appendLog, err)
		return
	}
	pub, err := publisher.New(targetCfg)
	if err != nil {
		failPublishTask(taskID, svc, req, scope, books, "", targetCfg, &logs, appendLog, err)
		return
	}
	mode := resolvePublishMode(req.Mode, targetCfg.ModeDefault)

	targetDir := ""
	targetPath := ""
	if scope == "full_site" || scope == "portal_only" {
		appendLog("[INFO] 开始构建门户首页")
		log, err := svc.Hugo.BuildMainSite(svc.SourceDir)
		appendLog(log)
		if err != nil {
			failPublishTask(taskID, svc, req, scope, books, "", targetCfg, &logs, appendLog, err)
			return
		}
	}
	if scope == "full_site" || scope == "single_book" {
		for _, book := range books {
			appendLog(fmt.Sprintf("[INFO] 开始构建书籍 %s", book))
			log, err := svc.Hugo.BuildBook(svc.SourceDir, book)
			appendLog(fmt.Sprintf("[BOOK] %s\n%s", book, log))
			if err != nil {
				failPublishTask(taskID, svc, req, scope, books, "", targetCfg, &logs, appendLog, err)
				return
			}
		}
	}
	switch scope {
	case "full_site":
		if err := svc.Hugo.MergeFullPackage(books); err != nil {
			appendLog("[ERROR] 合并全站构建产物失败")
			failPublishTask(taskID, svc, req, scope, books, "", targetCfg, &logs, appendLog, err)
			return
		}
		appendLog("[INFO] 已合并门户与书籍构建产物")
		targetDir = filepath.Join(svc.System.BuildTempRoot, "full_package")
	case "single_book":
		targetDir = filepath.Join(svc.System.BuildTempRoot, "book_cache", books[0])
		targetPath = books[0]
	case "portal_only":
		targetDir = filepath.Join(svc.System.BuildTempRoot, "main_site_out")
	}

	result, err := pub.PublishDir(targetDir, publisher.PublishOptions{
		Mode:       mode,
		Scope:      scope,
		TargetPath: targetPath,
		Logf:       appendLog,
	})
	if err != nil {
		failPublishTask(taskID, svc, req, scope, books, targetDir, targetCfg, &logs, appendLog, err)
		return
	}
	record := saveRecord(svc, req, scope, books, mode, targetDir, targetCfg, strings.Join(logs, "\n"), "success", "", result)
	appendLog("[INFO] 发布完成")
	svc.Tasks.Finish(taskID, publishtask.StatusSuccess, record.PublicURL, "")
}

func resolvePublishTarget(svc Services, targetType string, targetID string) (*configloader.PublishTargetConfig, error) {
	if targetType == "s3" && targetID == "system-default" {
		return &configloader.PublishTargetConfig{
			ID:               "system-default",
			Name:             "系统默认 S3",
			Type:             "s3",
			Enabled:          true,
			ModeDefault:      "incremental",
			Bucket:           svc.System.S3.DefaultBucketName,
			Region:           svc.System.S3.Region,
			Endpoint:         svc.System.S3.Endpoint,
			AccessKeyID:      svc.System.S3.AccessKeyID,
			SecretAccessKey:  svc.System.S3.SecretAccessKey,
			SitePublicDomain: svc.System.S3.SitePublicDomain,
			BasePrefix:       "/",
			CacheHTML:        svc.System.S3.CacheHTML,
			CacheStatic:      svc.System.S3.CacheStatic,
		}, nil
	}
	targetCfg, err := svc.Targets.Detail(targetType, targetID)
	if err != nil {
		return nil, err
	}
	if !targetCfg.Enabled {
		return nil, fmt.Errorf("发布目标 %s 已被禁用", targetCfg.Name)
	}
	return targetCfg, nil
}

func saveRecord(
	svc Services,
	req publishStartRequest,
	scope string,
	books []string,
	mode string,
	tempOutputPath string,
	targetCfg *configloader.PublishTargetConfig,
	logs string,
	status string,
	errMsg string,
	result *publisher.PublishResult,
) *configloader.PublishRecord {
	record := &configloader.PublishRecord{
		PublishingTime:  time.Now().Format("2006-01-02 15:04:05"),
		PublishingType:  "publish",
		PublishingScope: scope,
		PublishMode:     mode,
		BuildBooks:      normalizePublishBooks(books),
		TempOutputPath:  tempOutputPath,
		FullLog:         logs,
		Status:          status,
		ErrorMsg:        errMsg,
	}
	if targetCfg != nil {
		record.PublishingTargetType = targetCfg.Type
		record.PublishingTargetID = targetCfg.ID
		record.PublishingTargetName = targetCfg.Name
		record.TargetConfigSnapshot = map[string]any{
			"id":          targetCfg.ID,
			"name":        targetCfg.Name,
			"type":        targetCfg.Type,
			"mode":        targetCfg.ModeDefault,
			"target_dir":  targetCfg.TargetDir,
			"remote_dir":  targetCfg.RemoteDir,
			"bucket":      targetCfg.Bucket,
			"base_prefix": targetCfg.BasePrefix,
		}
		record.S3Bucket = targetCfg.Bucket
		record.S3Prefix = targetCfg.BasePrefix
	}
	if result != nil {
		record.PublicURL = result.PublicURL
		record.BackupPath = result.BackupPath
		record.BackupCreatedAt = result.BackupCreatedAt
		record.PublishedFiles = result.Files
	}
	_ = svc.Records.Save(record)
	return record
}

func resolvePublishMode(requestMode string, defaultMode string) string {
	mode := strings.TrimSpace(requestMode)
	if mode != "" {
		return mode
	}
	mode = strings.TrimSpace(defaultMode)
	if mode != "" {
		return mode
	}
	return "incremental"
}

func failPublishTask(
	taskID string,
	svc Services,
	req publishStartRequest,
	scope string,
	books []string,
	tempOutputPath string,
	targetCfg *configloader.PublishTargetConfig,
	logs *[]string,
	appendLog func(string),
	err error,
) {
	appendLog("[ERROR] " + err.Error())
	defaultMode := ""
	if targetCfg != nil {
		defaultMode = targetCfg.ModeDefault
	}
	mode := resolvePublishMode(req.Mode, defaultMode)
	saveRecord(svc, req, scope, books, mode, tempOutputPath, targetCfg, strings.Join(*logs, "\n"), "fail", err.Error(), nil)
	svc.Tasks.Finish(taskID, publishtask.StatusFailed, "", err.Error())
}

func normalizePublishBooks(books []string) []string {
	result := make([]string, 0, len(books))
	seen := map[string]struct{}{}
	for _, book := range books {
		book = strings.TrimSpace(book)
		if book == "" {
			continue
		}
		if _, exists := seen[book]; exists {
			continue
		}
		seen[book] = struct{}{}
		result = append(result, book)
	}
	return result
}

func respondFileResult(c *gin.Context, data any, err error) {
	if err == nil {
		Success(c, data)
		return
	}
	respondFileOnly(c, err)
}

func respondFileOnly(c *gin.Context, err error) {
	if err == nil {
		Success(c, gin.H{"ok": true})
		return
	}
	switch {
	case fsmanager.IsIllegalPath(err):
		Fail(c, 2003, "非法路径")
	case fsmanager.IsNotExist(err), os.IsNotExist(err):
		Fail(c, 2001, "目标文件不存在")
	default:
		Fail(c, 2002, err.Error())
	}
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": data})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg, "data": nil})
}

func writeSSE(c *gin.Context, event string, payload any) {
	raw, _ := json.Marshal(payload)
	_, _ = c.Writer.Write([]byte("event: " + event + "\n"))
	_, _ = c.Writer.Write([]byte("data: " + string(raw) + "\n\n"))
}
