# doc-editor-sd

一个基于 Go + React + Hugo 的文档编辑与发布后台。项目提供浏览器内文档管理、书籍目录维护、附件上传、Hugo 构建和静态站点发布能力，适合拿来搭企业内部知识库或文档门户。

## 功能概览

- 浏览器内编辑文档、目录、配置文件
- 按书籍维度管理 `source_root` 内容
- 支持附件上传和 S3 预签名直传
- 一键发布单本书或整站
- 自动生成发布记录，支持任务状态和日志查看
- 使用 Hugo 生成主站和书籍站点

## 项目结构

```text
.
├── web/                     # 前端编辑台，React + Molecule + Monaco
├── service/                 # 后端服务，Gin + Hugo 构建 + S3 发布
│   ├── cmd/                 # 服务入口
│   ├── config/              # 配置文件
│   ├── global_theme/        # Hugo 主题与模板
│   ├── internal/            # 后端核心实现
│   ├── source_root/         # 文档源数据
│   ├── static_resources/    # 前端构建产物，由后端直接托管
│   └── bin/                 # Hugo 可执行文件目录
├── release/                 # 打包输出目录
└── build_script.sh          # 前后端构建与 release 打包脚本
```

## 环境要求

- Go `1.26+`
- Node.js `18+`
- npm
- Hugo Extended

## Hugo 下载说明

这个项目依赖的是 `Hugo Extended`，不是普通版 Hugo。建议优先使用官方地址下载：

- 官方安装文档：<https://gohugo.io/installation/>
- 官方 Releases：<https://github.com/gohugoio/hugo/releases>

我查到 Hugo 官方当前展示的最新版本是 `v0.163.2`，发布时间是 `2026-06-15`。如果你在 Linux amd64 环境运行，可以在 Releases 页面选择类似下面的资产包：

- `hugo_0.163.2_linux-amd64.tar.gz`
- 或 `hugo_0.163.2_linux-amd64.deb`

下载后请把二进制放到下面这个位置，并命名为：

```bash
service/bin/hugo-extended
```

如果你想自定义路径，也可以改 [service/config/system.yaml.example](/www/wwwroot/CmsCode/service/config/system.yaml.example:3) 里的 `hugo_bin_path`。

## 快速开始

### 1. 安装前端依赖

```bash
cd web
npm install
```

### 2. 准备配置

复制示例配置并按你的环境修改：

```bash
cd /www/wwwroot/CmsCode/service
cp config/system.yaml.example config/system.yaml
```

重点配置项：

- `auth.admin_username` / `auth.admin_password`
- `s3.endpoint`
- `s3.access_key_id`
- `s3.secret_access_key`
- `s3.default_bucket_name`
- `s3.region`
- `s3.site_public_domain`
- `s3.img_cdn_domain`

站点展示相关配置在 [service/config/site_global.yaml](/www/wwwroot/CmsCode/service/config/site_global.yaml:1)。

### 3. 启动服务

```bash
cd /www/wwwroot/CmsCode/service
go run ./cmd
```

默认监听：

- `http://127.0.0.1:8080`

登录页：

- `http://127.0.0.1:8080/login.html`

## 构建前端

```bash
cd /www/wwwroot/CmsCode/web
npm run build
```

前端产物会输出到 `web/dist`，发布脚本会自动复制到 `service/static_resources`。

## 打包 release

在仓库根目录执行：

```bash
./build_script.sh
```

这个脚本会完成：

- 安装前端依赖（如缺失）
- 构建前端静态资源
- 编译 Go 服务
- 复制 Hugo、配置、主题、文档源文件
- 生成 `release/` 目录

打包完成后可使用：

```bash
cd release
./verify.sh
./run.sh
```

## 主要接口能力

后端提供的核心能力包括：

- `/api/login` / `/api/logout`
- `/api/fs/site/root-tree`
- `/api/fs/book/list`
- `/api/fs/file/content`
- `/api/fs/file/save`
- `/api/fs/file/new`
- `/api/fs/file/remove`
- `/api/fs/file/rename`
- `/api/fs/get-s3-upload-params`
- `/api/publish/full-site`
- `/api/publish/single-book`
- `/api/publish/task/status`
- `/api/publish/record/list`

## 文档数据说明

文档源目录默认在：

```text
service/source_root/
```

其中：

- `_site_meta.yaml` 管理书籍列表和排序
- 每本书都有自己的 `book_meta.yaml`
- Hugo 内容一般位于各书籍目录下的 `content/`
- 公共静态资源位于 `service/source_root/global_static/`

## 开发备注

- 后端启动时会校验 Hugo 可执行文件和 S3 配置
- 发布临时目录在 `service/build_temp/`
- 发布记录默认写入 `service/publish_records/`
- 当前仓库中的配置与域名均为演示数据，不是生产凭据

## 参考

- Hugo 官方安装文档：<https://gohugo.io/installation/>
- Hugo 官方 Releases：<https://github.com/gohugoio/hugo/releases>
