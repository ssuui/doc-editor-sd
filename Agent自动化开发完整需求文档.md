# AI Agent 自动化开发完整需求文档 V1.8
## 文档基础信息
1. 项目全称：多书籍门户Markdown文档发布系统
2. 文档用途：独立完整交付AI开发Agent，无任何上下文依赖，所有需求、约束、交互、接口、目录、配置全部完整内置，可直接全栈一次性开发
3. 核心产品定位
    1. Web仿VSCode Molecule在线Markdown编辑器，前后端工程完全分离；前端独立React+TS工程编译，编译产物内置嵌入Golang Gin后端静态资源统一分发
    2. 两级静态站点架构：顶层门户首页聚合全部书籍卡片，source_root一级子文件夹为独立书籍子站点
    3. 静态页面构建采用独立Hugo Extended二进制程序预生成纯静态HTML，SEO友好，放弃Docsify、VitePress
    4. 全站一键发布/单本书增量发布，构建产物自动批量上传兼容S3协议对象存储
    5. 图片上传采用**前端直传S3预签名URL方案**，图片全程不经过后端服务器，零服务器磁盘占用，适配小容量服务器部署；编辑器粘贴、按钮双上传交互
    6. 全系统无任何数据库：系统配置、书籍元数据、发布历史、站点信息全部使用本地YAML文件持久化
4. 全局硬性规则（本文档完整包含全部约束，无需查阅历史对话）
    1. 鉴权：32位随机字符串简易Token，禁用JWT；全局仅支持单设备独占登录，新登录会话直接失效旧会话
    2. 并发安全：内存文件读写RWMutex锁，所有文件写入、删除、重命名串行执行，杜绝多标签并发编辑覆盖文件
    3. 接口强制规范：**所有HTTP请求固定返回HTTP 200状态码**，业务成功/失败通过JSON内部自定义code区分，禁止使用401/404/500等HTTP错误状态码
    4. 图片上传规则：前端获取S3预签名PUT地址，浏览器直传图片至对象存储，后端不接收、不缓存、不落地任何图片二进制文件
    5. Token仅存储Go程序内存，服务重启全部失效；Token固定有效期15天
    6. 系统仅单固定管理员账号，账号密码配置在system.yaml，无多用户、权限管理页面
5. 全局统一接口返回结构体（全接口强制遵守，无例外）
```json
{
  "code": int,
  "msg": string,
  "data": any|null
}
```
### 全局业务状态码完整总表（固定不可修改）
| 业务code | 业务含义 |
| ---- | ---- |
| 0 | 操作成功 |
| 1001 | 鉴权失败：未登录 / Token过期 / 已在其他设备登录 |
| 1002 | 登录失败：账号或密码错误 |
| 2001 | 文件操作失败：目标文件不存在 |
| 2002 | 文件操作失败：磁盘读写权限异常、文件损坏 |
| 2003 | 路径非法：拦截路径逃逸`../`等攻击 |
| 3001 | 站点构建失败：Hugo进程执行报错、模板缺失、构建超时 |
| 4001 | 对象存储通用异常：S3配置错误、上传权限、网络异常、预签名生成失败 |
| 9999 | 服务内部未知异常 |

## 全局完整技术栈约束
| 分层 | 技术选型 | 强制约束细则 |
| --- | --- | --- |
| 前端工程 | React + TypeScript + @dtinsight/molecule | 1. 独立文件夹完整工程，单独npm编译；<br>2. 裁剪无用内置模块，禁止调用浏览器本地File System API；<br>3. 图片采用预签名直传S3，二进制不经过后端；<br>4. 所有业务操作通过HTTP接口请求Go后端 |
| 后端服务 | Golang 1.20+ + Gin Web框架 | 1. 无任何数据库、Redis、SQLite；<br>2. Token、文件锁仅存储程序内存；<br>3. 所有接口统一返回HTTP 200，内置统一返回封装函数；<br>4. 托管前端编译后的静态dist产物，统一静态路由分发 |
| 静态站点构建 | Hugo Extended 独立二进制 | 1. 禁止将Hugo源码引入Go项目；<br>2. exec.Command拉起子进程调用，进程隔离，构建崩溃不影响Web服务；<br>3. 多平台二进制内置至bin目录 |
| 对象存储 | S3标准兼容协议 | AWS S3、阿里云OSS、腾讯COS、MinIO通用；支持生成PUT预签名URL用于前端直传图片 |
| 持久化方案 | YAML配置文件 + Markdown文本 + 静态外链图片 | 系统配置、书籍元信息、发布历史全部落地本地磁盘YAML，无数据库；图片全部存储S3，本地不存放图片 |
| 鉴权方案 | 32位随机字符串Token | 无加密、无JWT；全局唯一登录会话，同一时间仅一台设备可操作系统 |
| 并发安全 | 内存RWMutex文件锁 | 文件读取加共享读锁，写入/删除/重命名加独占写锁 |
| 图片上传方案 | S3预签名直传 | 后端仅生成临时上传凭证，不接收图片二进制，零服务器磁盘占用 |

# 第一部分 项目整体物理目录完整规范
## 根目录总结构
```
doc-publish-server-root/
├─ web/                              # 独立完整前端工程（单独开发、单独编译）
│  ├─ package.json
│  ├─ tsconfig.json
│  ├─ src/
│  │  ├─ extensions/                 # 三大自定义业务插件（文件管理、MD编辑增强、发布模块）
│  │  │  ├─ FileManagerExtension.tsx
│  │  │  ├─ MarkdownEditorExtension.tsx # 内置图片粘贴、按钮上传、S3预签名直传交互
│  │  │  └─ PublishExtension.tsx
│  │  ├─ request/                    # 全局请求拦截、Token封装、S3预签名接口请求
│  │  ├─ views/
│  │  └─ App.tsx
│  ├─ public/
│  │  └─ login.html                  # 独立登录静态页面，无Molecule依赖
│  └─ dist/                          # npm run build编译产物，编译完成自动复制到Go后端资源目录
├─ service/                          # Golang Gin后端完整服务项目
│  ├─ bin/
│  │  └─ hugo-extended               # Hugo官方预编译二进制，服务启动自动校验存在与执行权限
│  ├─ global_theme/                  # 两套全局Hugo站点模板，所有书籍共享
│  │  ├─ main-site-template/         # 顶层门户首页聚合模板（书籍卡片、sitemap、全站导航）
│  │  └─ book-site-template/         # 单本书文档阅读模板（侧边目录、404、站内搜索）
│  ├─ config/
│  │  ├─ system.yaml                 # 系统底层全局配置（端口、登录账号、S3完整参数、预签名时效、路径、编辑器限制）
│  │  └─ site_global.yaml            # 门户展示配置（标题、LOGO、卡片布局、SEO开关）
│  ├─ source_root/                   # 用户文档源码根目录（仅存放MD、yaml、全站公共css/svg小静态资源，**不存放图片**）
│  │  ├─ _site_meta.yaml             # 全站书籍清单、排序权重、首页显隐控制
│  │  ├─ index.md                    # 门户自定义首页（可选，缺失自动生成汇总页）
│  │  ├─ global_static/              # 全站极小体积公共静态资源：LOGO、全局CSS、矢量图标（无图片）
│  │  ├─ book_01_前端开发手册/       # 一级子文件夹 = 单本独立书籍子站点
│  │  │  ├─ book_meta.yaml           # 本书展示元数据：书名、封面S3外链、简介、标签、版本
│  │  │  ├─ hugo.toml                # 本书独立Hugo构建参数配置
│  │  │  ├─ content/                 # Hugo标准MD章节文档目录，图片全部使用S3外链
│  │  │  └─ static/                  # 仅存放本书极小体积css/svg，不存放图片文件
│  │  ├─ book_02_运维规范/
│  │  └─ book_03_产品流程/
│  ├─ build_temp/                    # Hugo构建临时产物目录，后台定时自动清理
│  │  ├─ main_site_out/              # 顶层门户单独构建输出目录
│  │  ├─ book_cache/                 # 单本书独立构建缓存目录
│  │  └─ full_package/               # 门户+所有书籍合并后的完整站点包（S3上传源目录，不含图片）
│  ├─ publish_records/              # 发布记录存储目录，每次发布生成独立YAML记录文件
│  ├─ static_resources/              # Go内置前端静态资源目录
│  │  ├─ login.html
│  │  └─ index.html + js/css/asset   # web/dist编译产物全部复制至此，Gin统一托管分发
│  ├─ cmd/
│  │  └─ main.go                     # Go服务统一程序入口
│  └─ internal/                      # Go后端模块化业务代码
│     ├─ configloader/               # YAML配置加载、校验、写入统一工具包
│     ├─ filelock/                   # 文件并发读写锁模块（解决多标签并发保存覆盖）
│     ├─ auth/                        # 简易随机Token鉴权、单会话独占、Gin鉴权中间件
│     ├─ fsmanager/                  # 文件CRUD、目录树扫描、**S3预签名生成接口**
│     ├─ hugobuilder/                # Hugo二进制调用、门户首页自动生成、产物合并逻辑
│     ├─ s3uploader/                 # S3兼容对象存储批量上传模块（仅上传Hugo构建静态页面，不处理图片）
│     ├─ recordstore/                # 发布记录YAML读写、历史记录查询
│     └─ api/                        # Gin路由控制器、接口分组、全局统一返回封装工具
└─ build_script.sh                   # 一键编译脚本：先编译前端工程，自动复制dist产物到service/static_resources，再编译Go后端二进制
```
## 目录初始化开发执行步骤（Agent优先执行）
1. 严格按照上方完整树形结构创建全部空文件夹；
2. web 初始化独立React+TS+Molecule工程，内置独立login.html登录页面，MarkdownEditorExtension插件预留图片上传交互逻辑；
3. service/config 目录生成默认完整配置模板 system.yaml、site_global.yaml；
4. service/global_theme 创建两套空白Hugo模板目录骨架；
5. service/bin 预留hugo-extended二进制存放位置，服务启动自动校验文件存在与执行权限；
6. service/internal 创建所有业务模块空目录，生成基础代码骨架；
7. 编写 build_script.sh 一键编译脚本，实现前端编译产物自动拷贝至Go静态资源目录；
8. 明确约束：source_root、build_temp、static_resources所有目录**禁止存放任何图片文件**，图片仅永久存储S3。

# 第二部分 全部YAML配置文件完整字段规范
## 2.1 service/config/system.yaml 系统底层完整配置（新增S3预签名全套参数）
```yaml
# HTTP Web服务监听端口
http_port: 8080
# 用户文档源码根目录相对路径（Go后端内路径）
source_root_path: "./source_root"
# Hugo二进制文件存放路径
hugo_bin_path: "./bin/hugo-extended"
# 全局公共Hugo模板目录路径
global_theme_path: "./global_theme"
# Hugo构建临时根目录
build_temp_root: "./build_temp"
# 发布记录YAML文件存储目录
publish_record_path: "./publish_records"
# 临时构建目录自动清理周期，单位：小时
temp_clean_interval: 24
# 单次Hugo构建任务最大超时时间（秒），超时强制杀死子进程
build_task_timeout: 300
# ========== 登录鉴权完整配置（简易随机Token，无JWT） ==========
auth:
  # 系统唯一固定管理员账号
  admin_username: "admin"
  # 系统固定登录密码，明文存储配置文件
  admin_password: "AtlasDocs_2026!"
  # Token有效时长，单位小时，15天 = 360小时
  token_expire_hours: 360
# ========== S3兼容对象存储全局配置（含图片预签名直传全套参数） ==========
s3:
  endpoint: ""
  access_key_id: ""
  secret_access_key: ""
  default_bucket_name: "atlas-doc-portal-1300012345"
  region: ""
  # S3静态站点对外访问域名（文档页面域名）
  site_public_domain: "docs.atlaslab.example"
  # 图片资源CDN访问域名（前端渲染图片使用）
  img_cdn_domain: "assets.atlaslab.example"
  # 图片在桶内存储根前缀
  img_store_prefix: "book-res/"
  # 前端直传PUT预签名URL有效时长（分钟）
  presign_put_expire_min: 15
  # 静态页面缓存策略
  cache_html: "max-age=600"
  cache_static: "max-age=86400"
# ========== Molecule编辑器上传文件限制（仅限制图片，后端不接收二进制） ==========
editor_limit:
  max_file_mb: 20
  allow_img_ext: [".png", ".jpg", ".jpeg", ".gif", ".webp"]
```
## 2.2 service/config/site_global.yaml 门户全局展示配置
```yaml
# 门户首页大标题
site_title: "企业内部技术文档门户"
# 全站LOGO资源路径（存放于source_root/global_static，仅svg/png小图标，大图使用S3外链）
site_logo: "/global_static/logo.svg"
# 页面底部统一页脚文案
footer_text: "文档自动发布系统 | Hugo静态生成 + S3静态托管"
# 首页书籍卡片布局参数
home_card_layout:
  column_count: 3
  show_book_desc: true
  show_version_tag: true
# 全站顶部导航菜单
global_nav:
  - name: "门户首页"
    link: "/"
  - name: "全部书籍"
    link: "/#book-list"
# 自定义全局CSS样式文本
custom_global_css: ""
# SEO开关配置
enable_sitemap: true
enable_rss: false
```
## 2.3 service/source_root/_site_meta.yaml 全站书籍总清单（控制首页排序与显隐）
```yaml
# 书籍数组，weight数值越小，首页展示排序越靠前
book_list:
  - book_dir_name: "book_01_前端开发手册"
    weight: 10
    enable_home_show: true
  - book_dir_name: "book_02_运维规范"
    weight: 20
    enable_home_show: true
  - book_dir_name: "book_03_产品流程"
    weight: 30
    enable_home_show: false # 后台可编辑，门户首页不展示该书籍
```
## 2.4 service/source_root/{书籍文件夹}/book_meta.yaml 单本书籍元数据（封面使用S3图片外链）
```yaml
# 门户首页展示的书籍名称
display_name: "前端开发规范手册"
# 本书封面图片S3完整CDN外链
cover_img: "https://assets.atlaslab.example/book-res/book_01_前端开发手册/static-img/cover.png"
# 书籍简介，门户首页卡片展示
description: "前端工程化、组件开发、代码规范完整参考文档"
# 书籍分类标签
tags: ["前端", "Vue3", "工程化"]
# 书籍版本号
version: "v1.3.0"
# 是否在门户首页展示本书
visible_in_home: true
# 书籍阅读页面顶部自定义外链导航
extra_nav_links:
  - name: "内部Wiki"
    url: "https://wiki.atlaslab.example/frontend/standards"
```
## 2.5 service/publish_records/record_xxx.yaml 单条发布记录文件模板
```yaml
# 唯一记录ID，生成规则：时间戳_自增序号
record_id: "20260616_1735_001"
# 发布完整时间
publishing_time: "2026-06-16 17:35:42"
# 发布类型：full=全站完整发布 single=仅单本书增量发布
publishing_type: "full"
# 本次构建包含的书籍文件夹名称列表
build_books: ["book_01_前端开发手册", "book_02_运维规范"]
# 构建产物临时存放目录
temp_output_path: "./build_temp/full_package"
# S3文档页面上传目标存储桶
s3_bucket: "atlas-doc-portal-1300012345"
# S3文档页面上传根路径前缀
s3_prefix: "/"
# 最终对外可访问完整站点地址
public_url: "https://docs.atlaslab.example"
# 完整构建+上传全量日志文本
full_log: |
  [INFO] 开始渲染顶层门户首页
  [INFO] 构建书籍 book_01_前端开发手册 完成，耗时1.2s
  [INFO] 批量上传静态页面文件 326 个至S3文档桶
# 发布最终状态 success / fail
status: "success"
# 发布失败时记录错误详情，成功则为空字符串
error_msg: ""
```
## 配置模块开发强制要求
1. service/internal/configloader 统一封装所有YAML文件加载、字段校验、写入持久化函数；
2. 程序启动时自动加载全部配置，缺失必填字段直接阻断服务启动并打印清晰错误日志；
3. 所有读取、修改配置的逻辑必须调用configloader工具函数，禁止直接裸读写文件；
4. 配置文件不存在时，自动生成带默认值的空白模板文件；
5. S3预签名相关参数缺失直接阻断服务启动，提示补齐img_cdn_domain、img_store_prefix、presign_put_expire_min。

# 第三部分 Go后端完整开发需求（service）
## 后端模块开发执行顺序（Agent严格按顺序开发）
configloader → filelock → auth → fsmanager → hugobuilder → s3uploader → recordstore → api路由 → cmd/main入口
## 3.1 internal/configloader 配置加载工具包
1. 依赖 gopkg.in/yaml.v3，定义所有配置完整结构体并绑定yaml标签；
2. 提供统一加载函数：LoadSystemConfig、LoadSiteGlobalConfig、LoadSiteMeta、LoadBookMeta；
3. 通用SaveYaml写入函数，统一覆盖更新YAML文件；
4. 启动校验auth账号、密码、S3关键配置、图片预签名参数不能为空，非法配置直接退出程序。
## 3.2 internal/filelock 文件并发读写锁模块（解决多标签并发保存覆盖）
### 文件结构
internal/filelock/lock.go
### 核心实现逻辑
1. 全局并发安全存储：`fileLocks map[string]*sync.RWMutex`，map键为文件绝对磁盘路径；
2. 封装4个基础锁操作函数：
    - RLockFile(path string)：添加共享读锁，允许多请求并行读取同一文件
    - RUnlockFile(path string)：释放文件读锁
    - LockFile(path string)：添加独占写锁，同一文件同时仅允许1个写入操作
    - UnlockFile(path string)：释放文件写锁
3. 后台常驻协程定时清理长期闲置无操作的文件锁，避免内存持续堆积；
### 强制接入规则
service/internal/fsmanager 内所有文件读取操作必须包裹读锁；所有新建/保存/删除/重命名文件操作必须包裹独占写锁，无任何例外。
## 3.3 internal/auth 鉴权模块（简易32位随机Token + 全局单设备独占登录）
### 模块文件结构
internal/auth/
├─ token.go  # 随机Token生成、过期时间计算工具
├─ store.go  # 内存唯一会话存储、过期定时清理、新登录踢旧会话
└─ middleware.go # Gin全局鉴权中间件，统一返回HTTP 200+业务code
### 3.3.1 token.go 简易Token工具函数
1. 使用标准库 crypto/rand 生成32位大小写字母+数字随机字符串作为Token；
2. 函数 GenerateToken() string 输出全新随机Token；
3. 工具函数 CalcExpireTs(hours int64) int64：输入有效小时，返回过期时间戳（秒级）。
### 3.3.2 store.go 内存会话存储核心逻辑
1. 全局并发安全会话结构体定义：
```go
type Session struct {
    Token    string // 当前有效登录Token
    ExpireTs int64  // Token过期时间戳（秒）
}
var (
    globalSession *Session // 全局仅存在唯一一条有效会话
    mu            sync.RWMutex // 读写互斥锁，保证多请求并发安全
)
```
2. 核心对外方法：
   - SetNewSession(token string, expireTs int64)：创建新登录会话，直接覆盖原有会话，旧Token立即失效（实现单设备独占登录）
   - IsValidToken(token string) bool：校验传入Token与内存全局会话完全匹配、且当前时间未过期
   - ClearSession()：清空全局会话，所有设备Token失效（登出操作）
3. 后台常驻定时协程：每1小时执行一次会话过期校验，会话过期自动清空globalSession。
### 3.3.3 middleware.go 鉴权中间件（严格遵守全接口HTTP 200规范）
1. 接口白名单放行：`POST /api/login` 无需Token校验；
2. 静态页面跳转逻辑：
    - 访问根路径 `/`：无有效会话时302重定向至 `/login.html`；存在有效会话则返回编辑器index.html静态页面；
    - 访问 `/login.html` 直接返回登录静态页面，不做鉴权拦截；
3. 所有 `/api/*` 业务接口统一校验流程：
    1. 读取请求Header `Authorization: Bearer {token}`；
    2. Header不存在、token为空、token与内存全局会话不匹配、token已过期，统一调用Fail工具函数返回HTTP 200，JSON `code:1001`；
4. 中间件全程不返回401、403等HTTP错误状态码，严格遵循全局200约束。
### 3.3.4 登录接口 POST /api/login
#### 请求Body JSON
```json
{
  "username": "admin",
  "password": "AtlasDocs_2026!"
}
```
#### 业务执行流程
1. 读取system.yaml内配置的admin_username、admin_password进行字符串比对；
2. 账号密码不匹配：返回HTTP 200，`code:1002`，data为null；
3. 账号密码匹配成功：
    - 生成32位随机Token；
    - 按配置token_expire_hours计算过期时间戳；
    - 调用SetNewSession覆盖全局会话，已登录设备直接失效；
    - 返回HTTP 200，`code:0`，data携带token字符串；
#### 成功返回示例
```json
{
  "code": 0,
  "msg": "登录成功",
  "data": {
    "token": "Abc123Xyz789...32位随机字符串"
  }
}
```
#### 登录失败返回示例
```json
{
  "code": 1002,
  "msg": "账号或密码错误",
  "data": null
}
```
### 3.3.5 登出接口 POST /api/logout
1. 请求Header携带有效Bearer Token；
2. 调用ClearSession清空内存全局会话；
3. 返回HTTP 200，`code:0`；前端接收后清空本地localStorage Token并跳转登录页。
## 3.4 internal/fsmanager 文件管理模块（接口统一前缀 /api/fs，新增图片预签名接口）
### 安全强制约束
所有文件操作路径严格限制在 source_root_path 目录内，拦截 `../` 路径逃逸字符，非法路径返回 `code:2003`，HTTP 200。
### 全部接口清单（统一HTTP 200返回格式）
1. `GET /api/fs/site/root-tree`：读取顶层source_root完整目录树JSON，供给Molecule渲染全局文件树
2. `GET /api/fs/book/list`：读取source_root/_site_meta.yaml，返回全部书籍文件夹、排序权重、首页显隐状态
3. `GET /api/fs/book/tree?bookDirName=xxx`：返回单本书内部完整目录树（content、static、yaml、toml配置文件）
4. `GET /api/fs/file/content?path=xxx`：读取指定文件文本内容，操作前加读锁
5. `PUT /api/fs/file/save`：接收path、content字段，写入磁盘保存，操作前后加写锁
6. `POST /api/fs/file/new`：接收type(file/folder)、path，新建文件/文件夹，加写锁
7. `DELETE /api/fs/file/remove`：删除指定文件/文件夹，加写锁
8. `PATCH /api/fs/file/rename`：接收原路径、新名称，重命名文件/文件夹，加写锁
9. `GET /api/fs/get-s3-upload-params?bookDirName=xxx`：【新增图片直传专用接口】生成S3 PUT预签名URL，无任何文件IO操作
#### /api/fs/get-s3-upload-params 接口完整规范
请求入参URL参数：bookDirName 当前编辑书籍文件夹名称
业务逻辑：
1. 校验Token有效（鉴权中间件统一拦截code=1001）；
2. 读取system.yaml全部S3图片配置；
3. 生成唯一图片存储路径：`{img_store_prefix}{bookDirName}/static-img/{时间戳}_{32位随机串}.{后缀}`；
4. 调用S3 SDK生成PUT预签名URL，有效期取自presign_put_expire_min配置；
5. 返回完整上传参数给前端，后端**不接收、不缓存、不落地任何图片二进制**；
成功返回data结构：
```json
{
  "put_url": "https://bucket-endpoint/book-res/book_01_前端开发手册/static-img/1781642342_xxxx.png?签名参数",
  "cdn_img_url": "https://assets.atlaslab.example/book-res/book_01_前端开发手册/static-img/1781642342_xxxx.png"
}
```
异常返回规则（全部HTTP 200）
- S3签名生成失败、密钥错误：`code:4001`
- 书籍目录名称非法路径逃逸：`code:2003`
### 其余文件接口异常返回规则（全部HTTP 200）
- 文件不存在：`code:2001`
- 磁盘读写权限异常：`code:2002`
- 路径非法逃逸：`code:2003`
## 3.5 internal/hugobuilder Hugo构建核心模块
### 强制底层规则
1. 仅调用独立hugo-extended二进制，通过exec.Command拉起子进程，构建进程与Gin Web服务完全隔离；
2. 捕获子进程stdout、stderr日志，实时流式输出至前端发布日志面板；
3. 构建超时触发配置build_task_timeout，超时强制kill子进程，避免服务卡死。
### 三大核心业务逻辑
#### 逻辑1：顶层门户首页自动构建
1. 读取_site_meta.yaml、site_global.yaml、每本书book_meta.yaml，聚合所有书籍卡片展示信息；
2. 判断source_root/index.md是否存在：
   - 文件存在：直接使用该文件作为门户首页正文；
   - 文件不存在：程序自动生成汇总Markdown首页，包含所有启用展示的书籍卡片、跳转链接；
3. 执行Hugo命令使用main-site-template模板渲染门户静态HTML，输出至build_temp/main_site_out；
4. 自动复制source_root/global_static全站公共极小静态资源至门户构建目录（无图片）。
#### 逻辑2：单本书籍独立构建执行命令模板
```bash
./bin/hugo-extended \
--source={source_root_path}/{bookDirName} \
--themesDir={global_theme_path} \
--theme=book-site-template \
--destination={build_temp_root}/book_cache/{bookDirName} \
--timeout={build_task_timeout}
```
#### 逻辑3：全站产物合并流程
1. 清空build_temp/full_package目录；
2. 复制门户构建产物至full_package根目录；
3. 循环遍历所有enable_home_show=true的书籍，复制每本书构建产物至full_package/{bookDirName}/子目录；
### 图片构建适配规则
MD内图片全部为完整S3 CDN外链，Hugo构建时直接原样输出HTML img标签，无需拷贝本地图片资源，大幅减少构建产物体积。
### 异常处理：Hugo进程执行报错、模板缺失、构建超时，统一返回`code:3001`，HTTP 200，返回完整错误日志。
## 3.6 internal/s3uploader S3对象存储上传模块
### 业务边界强制区分
1. 本模块仅处理**Hugo生成的静态页面、css、svg**上传至文档站点桶；
2. 用户上传图片完全由前端直传S3图片桶，本模块不处理任何图片二进制；
### 功能需求
1. 程序启动读取system.yaml S3配置，初始化兼容S3客户端；
2. 实现递归批量上传整个文件夹函数，根据文件后缀自动匹配标准Content-Type、缓存头；
3. 实时统计上传成功、失败文件数量，记录完整上传日志；
4. 网络超时、密钥错误、桶不存在等异常统一返回`code:4001`，HTTP 200，携带详细错误信息。
## 3.7 internal/recordstore 发布记录存储模块
1. 生成唯一record_id（时间戳+自增序号）；
2. 将发布全量信息序列化为YAML写入publish_records目录；
3. 提供接口读取全部历史发布记录（按发布时间倒序排列）；
4. 根据record_id读取单条发布完整日志与站点访问地址；
## 3.8 internal/api Gin路由控制器（全局统一返回封装）
### 全局统一返回工具函数（强制所有接口调用）
```go
// 操作成功，携带data数据
func Success(c *gin.Context, data any)
// 操作失败，传入业务code与提示msg，固定HTTP 200
func Fail(c *gin.Context, code int, msg string)
```
### 接口分组划分
1. 白名单分组：`POST /api/login` 跳过鉴权中间件
2. 基础鉴权分组：`POST /api/logout`、`/api/fs/*`、`/api/publish/*`、`GET /api/build/check-hugo`
### 发布相关完整接口（前缀 /api/publish）
1. `POST /api/publish/full-site` 全站完整构建+上传发布
    流水线步骤：
    1. 清空全部构建临时目录；
    2. 构建顶层门户首页；
    3. 循环批量构建所有启用展示的书籍；
    4. 合并门户+所有书籍产物至full_package；
    5. 调用s3uploader递归上传完整静态页面目录至S3文档桶根路径；
    6. 生成本次发布记录YAML存入publish_records；
    7. 返回完整构建上传日志、公网站点访问地址、code=0
2. `POST /api/publish/single-book?bookDirName=xxx` 单本书增量发布
    流水线步骤：仅构建指定书籍、仅上传对应S3文档桶子目录，不重建顶层门户，提升发布速度
3. `GET /api/publish/record/list` 获取全部历史发布记录列表
4. `GET /api/publish/record/detail?recordId=xxx` 获取单条发布完整日志详情
### 辅助检测接口
`GET /api/build/check-hugo`：前端页面加载时检测Hugo二进制文件是否存在、可执行；检测失败返回code=3001，前端禁用所有发布按钮并弹窗提示
### 静态资源路由配置
1. GET `/login.html`：返回service/static_resources/login.html登录页面
2. GET `/`：鉴权校验，无有效会话302重定向/login.html；存在有效会话返回编辑器index.html
3. 匹配所有静态资源路径（js/css/png/svg等），托管service/static_resources目录下全部前端编译产物
## 3.9 cmd/main.go Go服务程序入口开发步骤
1. 加载system.yaml、site_global.yaml全部系统配置；
2. 启动三条后台常驻定时清理协程：
    1. build_temp临时目录定时清理；
    2. auth模块过期会话定时清理；
    3. filelock模块闲置文件锁定时清理；
3. 初始化Gin引擎，注册全局auth鉴权中间件、全局统一返回封装中间件；
4. 依次注册所有api分组路由、静态资源托管路由；
5. 校验bin/hugo-extended二进制文件存在与执行权限，校验失败直接退出程序；
6. 启动HTTP Web服务，监听配置文件http_port端口。

# 第四部分 独立前端工程 web 完整开发需求（含图片直传S3全套UI交互）
## 4.1 Molecule框架强制裁剪规则（移除无用模块，减少打包体积）
### 完全移除、不引入、不打包内置组件
Terminal终端、Debug调试面板、Git版本管理、内置插件市场、任务面板、Problems问题面板
### 仅保留最小运行底座
1. VSCode风格工作台布局：左侧活动栏、资源管理器文件树、顶部多标签Monaco编辑器、底部状态栏；
2. Monaco编辑器MD编辑内核；
3. Molecule原生扩展插件系统（所有业务逻辑封装自定义扩展，不修改框架内核源码）；
4. Ctrl+Shift+P全局命令面板、全局通知弹窗、基础快捷键、浅色/深色主题切换。
## 4.2 三大自定义业务扩展插件（开发顺序）
### 插件1：全局文件管理扩展（对接后端/api/fs全接口）
1. 页面初始化请求 `/api/fs/site/root-tree` 拉取全站目录树，渲染左侧Molecule文件树；
2. 文件树右键菜单功能：新建MD文件、新建文件夹、删除、重命名；
3. 点击文件自动新开编辑器标签页，请求后端接口加载文件文本至Monaco；
4. 监听编辑器Ctrl+S保存快捷键，调用PUT /api/fs/file/save接口；
5. 顶部下拉选择组件，快速切换当前编辑书籍目录。
### 插件2：Markdown编辑增强扩展（核心：图片粘贴+按钮上传S3直传全套UI交互）
#### 基础功能
1. 开启Monaco MD语法高亮、代码块高亮、Mermaid流程图渲染支持；
2. 编辑器右侧分栏实时MD预览，同步展示当前编辑内容；
3. 悬浮侧边TOC目录，快速跳转文档内各级标题；
4. 编辑器顶部快捷工具栏：一键插入表格、有序/无序列表、内部文档跳转链接、**图片上传按钮**。
#### 图片上传两套完整UI交互（严格贴合Molecule工作台轻量交互风格，无厚重弹窗）
##### 交互1：编辑器内直接粘贴图片（截图/本地复制图片）
1. 用户在编辑器光标位置Ctrl+V粘贴图片二进制；
2. 编辑器立即触发异步逻辑，展示**内联轻量Loading提示**（光标下方灰色小字：「正在获取云存储上传凭证…」）；
3. 前端携带当前打开书籍目录名，请求后端`GET /api/fs/get-s3-upload-params`接口；
4. 拿到预签名put_url、cdn_img_url后，隐藏第一步Loading，替换为新Loading提示：「正在上传图片至云存储」；
5. 浏览器直接发起PUT请求，二进制图片直传S3，不经过后端服务器；
6. 上传成功：自动在当前光标位置插入标准Markdown图片语法 `![图片](cdn_img_url)`，Loading提示消失；
7. 上传失败：右下角Molecule原生通知弹窗红色提示，文案区分两种错误：
    - 凭证获取失败：「上传凭证获取失败，请检查登录状态后重试」
    - 直传S3失败：「图片上传至云存储失败，请检查网络或图片格式」
8. 限制逻辑：自动校验图片后缀、单张文件大小，超出editor_limit限制直接弹窗拦截，不发起上传请求。

##### 交互2：顶部工具栏图片按钮点击上传
1. 编辑器顶部工具栏增加图片图标按钮（图片标识）；
2. 点击唤起浏览器原生文件选择框，仅筛选白名单图片后缀png/jpg/jpeg/gif/webp；
3. 选中文件后，底部编辑器状态栏展示小型横向进度条，同步执行「获取预签名→直传S3」完整流程；
4. 进度条100%上传完成后，进度条自动消失，光标插入图片MD链接；
5. 文件超限、格式非法直接弹窗提示，终止上传流程。
#### 图片资源规范约束
1. 所有图片仅存储S3，本地前端、后端均不缓存、不落地图片文件；
2. MD预览、Hugo构建页面全部使用S3 CDN外链渲染图片；
3. 禁止将图片上传至后端服务器本地目录。
### 插件3：全站发布核心扩展（系统核心功能）
1. UI操作入口：
    - 顶部菜单栏「发布」下拉菜单；
    - 底部状态栏两个快捷按钮：【完整发布全站】、【发布当前书籍】；
2. 点击发布按钮弹出二次确认弹窗，展示本次构建范围；
3. 底部新增独立「发布日志」面板，使用SSE长连接流式实时接收后端Hugo构建、S3静态页面上传日志，逐行打印；
    - 发布成功：展示一键复制S3公网站点访问链接；
    - 发布失败：日志标红错误内容，清晰展示故障原因；
4. Ctrl+Shift+P命令面板注册两条快捷指令：发布完整全站站点、仅发布当前书籍；
5. 侧边独立「发布历史记录」面板：请求后端获取全部发布记录，列表展示发布时间、状态、访问链接；点击单条记录，底部日志面板加载本次完整历史日志。
## 4.3 独立登录页面 web/public/login.html
1. 极简登录表单：用户名输入框、密码输入框、登录提交按钮；
2. 点击登录发送POST /api/login请求，解析返回JSON内code字段；
    - code=0：将token存入浏览器localStorage，页面跳转根路径 `/`；
    - code=1002：弹窗提示账号或密码错误；
3. 无任何Molecule框架依赖，纯独立静态页面。
## 4.4 前端全局请求统一封装（适配后端全HTTP 200规范）
1. 封装统一请求工具，所有fetch/axios请求自动从localStorage读取token，携带请求Header `Authorization: Bearer ${token}`；
2. 统一解析后端返回JSON的code字段做业务判断：
    - code=0：正常处理data业务数据；
    - code=1001：清空localStorage内token，页面强制跳转 `/login.html`，弹窗提示「登录失效或已在其他设备登录」；
    - 其余非0错误code：弹窗展示msg字段提示用户；
3. Molecule编辑器状态栏新增【退出登录】按钮，点击调用POST /api/logout接口，后端清空会话，前端清除本地token跳转登录页。
## 4.5 前端编译与产物拷贝规则
1. 执行编译命令 `npm run build`，产物输出至 web/dist；
2. build_script.sh 自动将dist内所有文件（login.html、index.html、js、css、静态资源）完整复制到 service/static_resources 目录；
3. Go后端Gin静态路由直接托管 static_resources 目录，无需单独部署前端服务。
## 4.6 前端硬性交互约束
1. 全程纯浏览器Web运行，**禁止调用浏览器File System本地文件API**，无本地文件夹授权弹窗；
2. 所有保存、发布、获取上传凭证异步操作增加Loading遮罩，屏蔽重复点击提交；
3. 不做前端多标签并发编辑限制，依靠后端「全局单会话独占登录 + 文件读写锁」双层防护杜绝文件覆盖；
4. 图片上传全程二进制不经过后端，仅通过预签名URL直传S3，不设计后端中转上传逻辑。

# 第五部分 全链路业务自测完整流程（Agent开发完成后逐条自测）
## 流程1：服务启动自检流程
1. 执行build_script.sh完成前端编译、产物拷贝、Go后端编译；
2. 运行编译后的Go二进制程序；
3. 程序自动加载全部YAML配置，校验必填S3图片预签名参数；
4. 校验bin/hugo-extended二进制文件存在、具备执行权限；
5. 启动三条后台定时清理协程；
6. Gin Web服务启动，监听配置端口。
## 流程2：未登录访问校验
1. 浏览器打开 http://127.0.0.1:8080；
2. 浏览器无本地Token，后端鉴权中间件302重定向至/login.html登录页面；
3. 直接在地址栏访问任意/api接口，抓包HTTP状态为200，返回JSON code=1001。
## 流程3：单设备独占登录测试
1. 设备A输入system.yaml内正确账号密码登录，正常编辑、保存MD文档；
2. 设备B打开同一地址，输入相同账号密码完成登录；
3. 设备A再次执行任意文件保存、发布、获取图片上传凭证接口，返回code=1001，页面自动跳转登录页；
4. 同一时间仅后登录设备可正常操作系统。
## 流程4：图片粘贴/按钮上传S3直传完整测试
1. 登录进入编辑器，打开任意书籍MD文件；
2. 测试粘贴图片：复制本地图片，编辑器Ctrl+V粘贴，观察内联Loading，上传完成自动插入S3 CDN图片链接，本地服务器无任何图片生成；
3. 测试工具栏图片按钮上传：点击按钮选择图片，状态栏进度条展示上传进度，完成插入图片链接；
4. 断网、错误S3密钥场景测试，前端弹窗对应错误提示，后端无图片二进制接收日志；
5. MD右侧预览面板正常渲染S3外链图片。
## 流程5：并发文件保存防覆盖测试
1. 同一浏览器打开两个标签页，打开同一MD文档；
2. 两个页面快速连续按下Ctrl+S提交不同内容；
3. 后端filelock独占写锁串行执行两次写入，最终文件内容无丢失、无截断、无错乱覆盖。
## 流程6：全站完整发布业务流程
1. 编辑顶层source_root/index.md自定义门户首页；若删除该文件，执行发布时程序自动生成汇总全部书籍的首页；
2. 点击顶部【完整发布全站】确认弹窗；
3. 底部日志面板SSE实时流式输出Hugo构建、S3静态页面上传完整日志；
4. 构建上传完成，生成publish_records目录下全新YAML发布记录；
5. 页面展示可一键复制的S3公网站点访问地址；
6. 浏览器访问S3域名根路径，展示门户首页，点击书籍卡片正常跳转对应子站点静态文档页面，图片通过CDN外链正常加载，纯HTML可被搜索引擎抓取（SEO友好）。
## 流程7：单本书增量发布流程
1. 在编辑器打开任意书籍内MD文档；
2. 点击状态栏【发布当前书籍】按钮；
3. 后端仅构建当前书籍，不重建顶层门户首页；仅上传该书籍对应S3文档桶子目录，构建、上传速度大幅提升；
4. 生成独立发布记录，记录内仅包含当前书籍名称。
## 流程8：Token全部失效场景测试
1. 正常登录获取有效Token，编辑文档、上传图片；
2. 停止并重启Go后端服务：内存全局会话清空，原有Token全部失效；刷新页面自动跳转登录页；
3. Token超过配置15天有效期：后台定时协程自动清空会话，接口访问返回code=1001；
4. 点击编辑器状态栏【退出登录】：后端ClearSession清空会话，前端清除本地Token跳转登录页。
## 流程9：Hugo环境检测校验
1. 删除bin/hugo-extended二进制文件，重启Go服务，程序直接阻断启动并打印错误；
2. 前端页面加载时自动调用 /api/build/check-hugo，Hugo缺失时所有发布按钮置灰不可点击，弹窗提示缺失构建工具。

# 第六部分 全系统非功能性硬性开发约束（不可违反）
1. 零数据库强制约束：禁止引入MySQL、SQLite、Redis、LevelDB等任何数据库；系统配置、书籍元数据、发布记录全部存储YAML文件；Token、文件锁仅存放Go程序内存，不落地任何文件。
2. 构建引擎约束：仅通过exec.Command调用独立Hugo Extended二进制子进程；禁止将Hugo源码引入Go Module、禁止调用Hugo内部构建API；禁止自研MD转HTML、页面渲染、导航生成逻辑。
3. 鉴权约束：禁用JWT、加密签名Token方案，仅使用32位随机字符串简易Token；系统全局仅维护唯一有效登录会话，新登录强制使旧会话失效，杜绝多端同时编辑文件覆盖。
4. 接口全局强制约束：**全部API接口HTTP状态码固定返回200 OK**，业务成功、失败、异常完全依靠JSON内部code字段区分，禁止使用401/404/500等HTTP错误状态码。
5. 文件安全约束：所有文件读写操作必须接入filelock读写锁，写入操作独占串行执行，杜绝并发保存导致内容丢失、文件损坏。
6. 图片存储硬性约束：**禁止后端接收、缓存、落地任何图片二进制文件**，统一采用前端S3预签名直传方案；服务器所有目录不存放图片资源，图片永久存储对象存储CDN。
7. 全配置驱动约束：账号密码、Token过期时长、S3存储参数、图片预签名时效、页面布局参数、构建超时时间全部定义在system.yaml配置文件，禁止代码硬编码任何业务参数。
8. 前后端工程分离约束：前端为独立完整React+TS工程，单独编译；编译产物自动拷贝嵌入Go后端static_resources目录，由Go统一静态分发，不单独部署前端服务。
9. 轻量化部署约束：运行环境仅依赖编译后的Go二进制程序 + bin目录下Hugo二进制文件；无需NodeJS、Python、Java等额外运行时环境。
10. 无冗余功能约束：不实现思源笔记同类双链、图谱、块数据库、闪卡、多用户权限管理；系统仅提供三大核心能力：Web在线MD文档编辑、两级静态站点自动构建、一键S3静态站点发布 + 前端直传图片云存储。

# 第七部分 Agent开发完整交付产物清单
## 1. 前端工程 web 完整源码包
1. 完整React+TS+Molecule项目，已裁剪无用内置模块；
2. 三大自定义业务扩展插件完整实现：文件管理、MD实时预览（含图片粘贴/按钮直传S3全套UI交互）、发布日志+发布历史面板；
3. 独立无依赖login.html登录页面；
4. 全局请求拦截封装，自动携带Token、统一处理code=1001登录失效跳转；
5. SSE实时接收后端发布日志逻辑、状态栏登出按钮；
6. package.json编译脚本 npm run build，图片上传Loading、进度条、错误弹窗完整交互代码。
## 2. Go后端 service 完整源码包
1. 模块化分层完整代码：configloader / filelock / auth / fsmanager（含S3预签名接口） / hugobuilder / s3uploader / recordstore / api；
2. 全局统一HTTP 200返回封装工具、鉴权中间件、文件并发锁、Hugo进程管控、三大后台定时清理协程；
3. cmd/main.go完整服务入口；
4. 全套默认YAML配置模板 system.yaml、site_global.yaml（完整包含图片S3预签名参数）；
5. 预留bin/hugo-extended、global_theme两套Hugo模板目录骨架。
## 3. 配套脚本与资源
1. build_script.sh 一键编译脚本：自动编译前端、拷贝dist产物至Go静态目录、编译Go后端二进制；
2. 两套完整可直接使用的Hugo全局模板：main-site-template（门户首页聚合）、book-site-template（书籍文档阅读页）；
## 4. 配套文档资料
1. 完整接口文档：全部/api接口请求入参、返回JSON结构、业务code含义，新增图片预签名接口完整说明；
2. 部署操作文档：服务编译、Hugo二进制替换升级、S3图片桶CORS配置、system.yaml图片参数修改说明；
3. 全链路自测用例文档（登录独占、并发文件保存、图片直传S3、全站/单书发布、Token失效校验）。

# 第八部分 交付验收逐条校验标准（开发完成后Agent逐条自检）
1. 执行一键编译脚本，前端正常打包，产物自动复制至service/static_resources目录，Go后端可正常编译运行；
2. 浏览器访问8080端口，无Token时自动跳转独立login.html登录页面；输入配置内正确账号密码可正常进入Molecule仿VSCode编辑器；错误账号密码弹窗提示登录失败；
3. 多设备同时登录，后登录设备挤下线先登录设备，旧设备所有接口返回code=1001，自动跳转登录页；
4. 编辑器粘贴图片、点击工具栏图片按钮均可完成S3直传，服务器本地无任何图片文件生成，MD自动插入CDN图片链接，预览正常渲染；
5. 同一浏览器双标签并发保存同一MD，后端文件锁串行写入，文件内容无覆盖、丢失、截断；
6. 所有API接口抓包查看HTTP Status恒为200，业务成功/失败完全依靠JSON内code字段区分；
7. 删除顶层source_root/index.md后执行全站发布，程序自动生成汇总全部书籍的门户首页；
8. 完整发布全站、单本书增量发布功能正常，Hugo构建静态页面自动批量上传S3文档桶，公网域名可正常访问两级文档门户，图片CDN外链加载正常；
9. 重启Go后端服务后，所有原有Token全部失效，必须重新登录；
10. 登录账号、密码、Token15天有效期、S3文档桶/图片桶全部配置参数存储在system.yaml，代码无硬编码；
11. Hugo通过独立二进制子进程调用，构建进程崩溃不会导致Web编辑器服务宕机；
12. 系统运行目录无任何数据库文件生成，全部配置、发布记录仅存储YAML文件；
13. 页面全程无浏览器本地文件授权弹窗，所有文件读写操作由Go后端代理本地磁盘；图片上传不经过后端服务器，零磁盘占用；
14. 站点全部页面由Hugo预生成纯静态HTML，搜索引擎可完整抓取页面正文内容，满足SEO需求；
15. 编辑器状态栏存在退出登录按钮，点击清空后端会话与本地Token，跳转登录页面；
16. 侧边发布历史面板可查看全部历史发布记录，点击单条记录加载完整构建与上传日志；
17. S3图片桶配置缺少预签名相关参数时，服务启动直接阻断并打印提示信息；
18. 图片上传超限、格式非法、网络异常场景前端对应弹窗提示，后端无图片二进制接收日志。
