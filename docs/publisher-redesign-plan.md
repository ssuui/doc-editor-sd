# 发布器改版规划文档

## 1. 文档目标

基于当前项目的发布器实现，整理一份后续改版规划，重点覆盖：

- 重建索引入口与交互改版
- 发布入口、发布范围、发布模式重构
- 发布配置从系统配置解耦
- 发布记录增强为可追踪、可回看、可恢复
- 前后端接口、数据结构、实施阶段建议

本规划以当前仓库实现为基础，面向下一阶段开发落地。

## 2. 当前实现现状

### 2.1 前端现状

当前前端主逻辑集中在 [web/src/main.tsx](/www/wwwroot/CmsCode/web/src/main.tsx)：

- “重建 `_site_meta.yaml` / `_books_meta.yaml`” 入口位于左侧 Activity Bar 全局按钮区域，紧邻“退出登录”等按钮。
- 发布入口位于底栏状态栏，分为“完整发布”和“发布当前书籍”两个按钮。
- 搜索面板目前只有搜索相关能力，没有承载索引或发布操作。
- 退出登录已有二次确认弹窗。
- “关闭未保存文档”已有二次确认逻辑，但“重建索引”和“发布”还没有复用这套风险确认机制。

### 2.2 索引重建现状

当前后端索引重建逻辑在 [service/internal/fsmanager/service.go](/www/wwwroot/CmsCode/service/internal/fsmanager/service.go)：

- `RebuildSiteMeta()` 会扫描书籍目录后，直接整体重写 `source_root/_site_meta.yaml`
- `RebuildBooksMeta()` 会扫描书籍目录后，直接整体重写 `source_root/_books_meta.yaml`
- 权重从 `10, 20, 30...` 重新生成
- 原文件中的人工调整内容不会保留

也就是说，当前行为是“全量扫描 + 全量覆盖”，不支持：

- 仅补充新增书籍
- 保留现有顺序和首页显示配置
- 预览变更
- 安全确认

### 2.3 发布现状

当前发布主流程在 [service/internal/api/router.go](/www/wwwroot/CmsCode/service/internal/api/router.go) 与 [service/internal/s3uploader/service.go](/www/wwwroot/CmsCode/service/internal/s3uploader/service.go)：

- 目前只有两类发布动作：
  - 全站发布 `/api/publish/full-site`
  - 单书发布 `/api/publish/single-book`
- 发布后统一走 `svc.Uploader.UploadDir(...)`
- 当前上传器只有 S3 目标实现
- 发布目标配置完全依赖 `service/config/system.yaml` 中的 S3 配置
- 编辑器内附件上传也使用同一套系统级 S3 配置

当前主要问题有三类：

- 发布范围不够细，缺少“单独发布门户”
- 附件上传配置与站点发布配置耦合在一起
- 发布流程只有一种“上传覆盖”思路，没有明确区分增量与覆盖发布

### 2.4 发布记录现状

当前发布记录结构定义在 [service/internal/configloader/config.go](/www/wwwroot/CmsCode/service/internal/configloader/config.go)，记录保存逻辑在 [service/internal/recordstore/service.go](/www/wwwroot/CmsCode/service/internal/recordstore/service.go)。

当前记录仅包含：

- 发布时间
- 发布类型
- 书籍列表
- 临时输出目录
- S3 Bucket / Prefix
- Public URL
- 全量日志
- 状态 / 错误信息

当前未记录：

- 本次实际发布的文件清单
- 本地源文件到目标文件的映射关系
- 发布目标类型和配置快照
- 覆盖发布前的备份目录信息

因此当前无论是做“精细追踪”，还是做“从备份版本恢复”，数据基础都还不够。

## 3. 本次改版目标

### 3.1 总体目标

把发布器从“几个直接触发的按钮”升级成“带操作面板、可选择范围、可选择模式、可确认、可追踪、可恢复”的工作流。

### 3.2 体验目标

- 索引与发布入口统一收敛到搜索区域下方
- 点击入口后打开右侧操作侧栏，而不是直接执行
- 所有高风险动作都先二次确认
- 有未保存文档时，先拦截再确认
- 发布和索引都能选择不同模式
- 发布记录能回看、能定位、能查看备份位置

### 3.3 架构目标

- 附件上传继续保留系统 S3 配置
- 只有“发布目标配置”从系统配置中拆出
- 发布能力按目标类型插件化
- 发布记录具备文件级追踪和备份追踪能力

## 4. 改版方案总览

建议在搜索面板下方新增两个固定入口：

- `索引操作`
- `发布中心`

点击后不直接执行，而是打开统一样式的操作侧栏。

建议侧栏分为两类：

1. 索引侧栏
2. 发布侧栏

这样可以把原来分散在 Activity Bar、底栏状态栏中的动作收回到搜索区附近，入口更集中，也更符合“先选择策略，再执行”的操作模型。

## 5. 索引重建改版规划

### 5.1 入口调整

现状：

- 两个重建按钮在 Activity Bar，位置靠近退出登录

调整为：

- 在搜索面板下方增加 `索引操作` ICON 或按钮组
- 点击后打开“索引操作侧栏”
- 原 Activity Bar 中对应按钮移除

建议侧栏展示：

- 索引目标：
  - `_site_meta.yaml`
  - `_books_meta.yaml`
  - 两者一起处理
- 操作模式：
  - 增量补充新索引
  - 全量更新
  - 预览变更后更新

### 5.2 推荐的索引模式设计

#### 模式 A：增量补充新索引

这是最符合你描述、也最应该作为默认推荐项的模式。

行为建议：

- 扫描现有书籍目录
- 读取旧的 `_site_meta.yaml` 或 `_books_meta.yaml`
- 已存在的条目保持不动
- 新扫描到但旧文件中不存在的书籍，追加到末尾
- 新书默认分配新的权重
- `_site_meta.yaml` 中新书的 `enable_home_show` 默认按 `book_meta.yaml` 的 `visible_in_home` 推断

价值：

- 不破坏人工调整顺序
- 不覆盖已有首页显示策略
- 适合日常新增书籍后的维护

#### 模式 B：全量更新

行为建议：

- 按扫描结果重新生成完整索引
- 旧文件内容全部被新的扫描结果替换
- 权重重新按固定步长生成

适用：

- 初次初始化
- 历史索引混乱时重建
- 管理员明确希望“以文件系统为准”

#### 模式 C：预览变更后更新

这是建议补充的一项能力。

行为建议：

- 先生成 diff 预览
- 将变更分组展示：
  - 新增书籍
  - 缺失书籍
  - 顺序变化
  - `enable_home_show` 推断值
- 用户确认后再写入

#### 模式 D：清理失效条目

行为建议：

- 识别索引中存在，但目录中已不存在的书籍
- 允许只清理这部分失效项
- 不改动仍有效的旧条目

### 5.3 索引重建确认机制

索引重建必须加入二次确认，并且要兼容“有未保存文档”的场景。

建议流程：

1. 用户点击侧栏中的某个索引动作
2. 前端先检查是否有未保存文档
3. 如果存在未保存文档，弹出第一层确认
4. 用户继续后，再弹出第二层确认
5. 确认后才调用后端接口

建议提示：

- 第一层：当前有未保存文档，继续执行索引操作可能导致视图与磁盘内容不一致，是否继续？
- 第二层：确认执行“增量补充 / 全量更新 / 清理失效 / 预览后更新”？

### 5.4 索引后端改造建议

现有接口：

- `POST /api/fs/site/rebuild-meta`
- `POST /api/fs/site/rebuild-books-meta`

建议调整为模式化接口，例如：

- `POST /api/index/plan`
- `POST /api/index/apply`

请求体建议：

```json
{
  "mode": "append_new",
  "preview": true,
  "remove_missing": false,
  "targets": ["site_meta", "books_meta"]
}
```

建议后端抽出统一索引服务，例如：

- `service/internal/indexmanager/service.go`

### 5.5 索引数据策略建议

`_books_meta.yaml` 建议保留字段：

- `book_dir_name`
- `weight`

`_site_meta.yaml` 建议保留字段：

- `book_dir_name`
- `weight`
- `enable_home_show`

增量模式下：

- 已存在书籍完全保留
- 新书按最大 `weight + 10` 追加
- `_site_meta.yaml` 中 `enable_home_show` 默认读取 `book_meta.visible_in_home`

对于“索引中有，但实际目录不存在”的书籍，建议不要在增量模式下自动删除，而是：

- 在预览中标记为“失效项”
- 交给“清理失效条目”或“全量更新”处理

## 6. 发布中心改版规划

### 6.1 入口调整

现状：

- 底栏状态栏有“完整发布”“发布当前书籍”

调整为：

- 在搜索面板下方新增 `发布中心` ICON
- 点击后打开“发布操作侧栏”
- 底栏不再承担动作入口职责
- 底栏可以保留为“任务日志 / 实时状态”承载区

### 6.2 发布维度重构

建议把发布拆成三个维度：

1. 发布范围
2. 发布目标
3. 发布模式

#### 发布范围

- 发布整站
- 单独发布书籍
- 单独发布门户

这里“单独发布门户”应该成为独立范围，而不是继续隐含在“整站发布”里。

#### 发布目标

- 发布到 S3
- 发布到当前服务器目录
- 发布到 SSH / SFTP 远程目录

#### 发布模式

- 增量模式
- 覆盖模式

这样组合后，动作会变成：

- 整站 -> S3 -> 增量模式
- 书籍 -> 本地目录 -> 覆盖模式
- 门户 -> SFTP -> 增量模式

### 6.3 发布中心侧栏结构建议

建议侧栏分为五个区块：

#### 区块 1：发布范围

- 整站
- 当前书籍
- 门户

#### 区块 2：发布目标

- S3
- 目录
- SSH / SFTP

#### 区块 3：发布模式

- 增量模式
- 覆盖模式

#### 区块 4：目标配置

根据发布目标切换不同表单：

- S3 发布配置选择器
- 目录发布配置选择器
- SSH / SFTP 发布配置选择器

#### 区块 5：执行动作

- 保存配置
- 测试连接
- 开始发布
- 查看最近发布记录

### 6.4 二次确认机制

发布也必须加入二次确认，并且规则与索引类似。

建议流程：

1. 用户选择发布范围、发布目标、发布模式、配置
2. 检测未保存文档
3. 若存在未保存内容，先确认
4. 再弹出发布确认

确认弹窗建议展示：

- 发布范围
- 目标类型
- 目标名称
- 发布模式
- 目标目录或前缀
- 覆盖模式下是否创建备份目录

确认文案示例：

- 确认将“当前书籍”以“增量模式”发布到“S3 配置：生产环境”？
- 确认将“门户”以“覆盖模式”发布到“本地目录：/www/wwwroot/docs”，并先备份到 `bak/20260619_153000`？

## 7. 发布配置解耦设计

### 7.1 边界定义

这里先明确边界：

- 编辑器附件上传继续使用 `system.yaml` 里的系统 S3 配置
- 只有“发布目标配置”拆出来独立管理

也就是说，这次不改“上传附件走系统 S3”这件事。

### 7.2 为什么要拆发布配置

只拆“发布配置”也已经可以解决下面这些问题：

- 无法维护多个发布目标
- 无法为不同环境保存不同配置
- 无法支持目录发布和 SFTP 发布
- 无法给发布记录保存“配置快照”

### 7.3 建议配置目录

建议新增专门的发布配置目录：

```text
service/config/publish_targets/
```

下面按类型分文件保存：

```text
service/config/publish_targets/
├── s3/
│   ├── prod-main.yaml
│   └── staging.yaml
├── local/
│   ├── server-site.yaml
│   └── test-dir.yaml
└── sftp/
    ├── hk-prod.yaml
    └── backup.yaml
```

### 7.4 配置文件结构建议

#### S3 发布配置

```yaml
id: prod-main
name: 生产 S3
type: s3
enabled: true
bucket: your-bucket
region: ap-guangzhou
endpoint: https://cos.ap-guangzhou.myqcloud.com
access_key_id: xxx
secret_access_key: xxx
site_public_domain: docs.example.com
base_prefix: /
cache_html: max-age=600
cache_static: max-age=86400
mode_default: incremental
```

#### 本地目录发布配置

```yaml
id: local-main
name: 当前服务器站点目录
type: local_dir
enabled: true
target_dir: /www/wwwroot/docs
bak_dir: /www/wwwroot/docs/bak
mode_default: overwrite
```

#### SFTP 发布配置

```yaml
id: hk-prod
name: 香港服务器
type: sftp
enabled: true
host: 10.0.0.12
port: 22
username: deploy
password: ""
private_key_path: /www/wwwroot/CmsCode/service/config/keys/hk-prod.pem
remote_dir: /home/www/docs
remote_bak_dir: /home/www/docs/bak
mode_default: incremental
```

### 7.5 建议的数据模型

建议新增统一发布目标配置模型：

- `PublishTargetConfig`
- `PublishTargetType = s3 | local_dir | sftp`

并增加字段：

- `id`
- `name`
- `type`
- `enabled`
- `created_at`
- `updated_at`

## 8. 发布执行架构改造建议

### 8.1 建议抽象发布器接口

当前 `s3uploader.Service` 只负责 S3。建议升级成“发布目标适配器”模式。

示意：

```go
type Publisher interface {
    Validate() error
    PublishDir(localDir string, options PublishOptions) (*PublishResult, error)
    TestConnection() error
}
```

建议实现：

- `S3Publisher`
- `LocalDirPublisher`
- `SFTPPublisher`

### 8.2 建议的发布流程

1. 解析发布范围
2. 生成对应构建产物
3. 解析发布目标配置
4. 根据目标配置创建具体 Publisher
5. 根据发布模式执行“增量发布”或“覆盖发布”
6. 返回文件映射结果与备份信息
7. 保存发布记录
8. 推送任务状态

### 8.3 发布模式设计

#### 增量模式

这是你建议作为当前默认延续模式的方案，建议定义为：

- 检索本次构建产物中的文件
- 逐个发布到目标位置
- 目标中同路径文件直接覆盖
- 目标中本次未涉及的其它文件不处理

特点：

- 行为接近当前模式
- 风险较低
- 适合日常发布
- 不负责清理历史残留文件

#### 覆盖模式

这是你提出的更稳妥方案，建议作为高风险但可恢复的模式。

发布流程建议：

1. 计算本次发布影响的目标目录
2. 在目标目录下或配置指定位置创建 `bak` 目录
3. 以 `YYYYMMDD_HHMMSS` 创建版本子目录
4. 将本次要覆盖的旧目录整体移动到该备份版本目录下
5. 再把新产物发布到正式目录

示例：

```text
/www/wwwroot/docs/
├── bak/
│   └── 20260619_153000/
│       ├── portal/
│       └── b_the_road_less_traveled/
├── portal/
└── b_the_road_less_traveled/
```

这样做的好处是：

- 不再把“删除旧文件”作为主恢复手段
- 覆盖前保留了一份可追溯备份
- 后续如需恢复，可以直接从 `bak` 版本还原

#### 不同发布范围下的覆盖粒度

- 整站发布：备份门户目录和所有书籍目录
- 单独发布书籍：只备份当前书籍目录
- 单独发布门户：只备份门户目录

### 8.4 本地目录发布建议

本地目录发布建议优先落地你定义的两种模式：

- 增量模式：按文件覆盖，不清理其它文件
- 覆盖模式：发布前先移动旧目录到 `bak/时间版本`

这里建议先不要做复杂的“发布后自动删除旧文件”，而是把恢复路径建立在 `bak` 目录上。

### 8.5 SSH / SFTP 发布建议

建议命名上对用户展示为：

- `SSH / SFTP`

但技术实现上优先按 `SFTP` 落地，因为它天然适合文件上传、目录移动和备份目录管理。

建议支持：

- 密码登录
- 私钥登录
- 远程目录存在性检查
- 写权限测试
- 远程 `bak` 目录创建与版本目录移动

## 9. 发布记录增强设计

### 9.1 核心目标

发布记录不再只是“日志存档”，而要升级成“可追踪的发布快照”。

每条记录至少应知道：

- 发布了什么
- 发到了哪里
- 实际写了哪些文件
- 覆盖模式下旧文件被备份到了哪里

### 9.2 建议新增字段

当前 `PublishRecord` 建议扩展为：

- 记录基础信息
  - `record_id`
  - `publishing_time`
  - `publishing_scope`
  - `publishing_target_type`
  - `publishing_target_id`
  - `publishing_target_name`
- 构建信息
  - `build_books`
  - `temp_output_path`
- 配置快照
  - `target_config_snapshot`
- 结果信息
  - `public_url`
  - `status`
  - `error_msg`
  - `full_log`
- 模式与备份信息
  - `publish_mode`
  - `backup_path`
  - `backup_created_at`
- 文件映射
  - `published_files`

其中 `published_files` 建议至少包含：

```yaml
published_files:
  - source_rel_path: b_the_road_less_traveled/index.html
    target_path: s3://bucket/b_the_road_less_traveled/index.html
    target_key: b_the_road_less_traveled/index.html
    file_size: 18342
    checksum: sha256:xxxx
  - source_rel_path: portal/index.html
    target_path: /www/wwwroot/docs/portal/index.html
    file_size: 92831
```

### 9.3 为什么仍建议保存文件 MAP

虽然现在不把“删除某次发布文件”作为主能力，但文件 MAP 仍然值得保留，因为它还有这些用途：

- 记录本次到底发布了哪些文件
- 辅助排查增量模式下哪些文件被覆盖
- 辅助生成发布报告
- 为后续精细恢复保留依据

所以建议：

- “删除本次发布文件”不作为第一阶段主能力
- 但 `published_files` 仍然保留

### 9.4 恢复能力设计

建议把“删除”降级为可选扩展，把“恢复”作为主方案。

入口建议放在发布记录详情中：

- 查看本次发布文件
- 查看本次备份目录
- 从该备份版本恢复

覆盖模式下的恢复逻辑建议：

1. 读取该记录的 `backup_path`
2. 校验备份目录存在
3. 将当前正式目录移动到临时位置
4. 把 `backup_path` 下对应目录恢复到正式位置
5. 记录恢复日志

增量模式下由于没有完整备份，不建议承诺“一键恢复”。

## 10. 前端页面规划建议

### 10.1 搜索区下方新增操作区

建议在搜索区下方增加一个轻量操作组：

- `索引操作`
- `发布中心`

### 10.2 右侧操作侧栏

建议新增统一侧栏组件：

- `OperationDrawer`

子场景：

- `IndexOperationDrawer`
- `PublishOperationDrawer`
- `PublishRecordDrawer`

### 10.3 发布记录页建议

建议在发布中心内补齐：

- 发布记录列表
- 发布记录详情
- 查看日志
- 查看文件映射
- 查看备份路径
- 从备份版本恢复

列表筛选建议支持：

- 发布时间
- 发布目标类型
- 发布配置
- 发布范围
- 发布模式
- 状态

## 11. 后端接口建议清单

### 11.1 索引相关

- `POST /api/index/plan`
- `POST /api/index/apply`

### 11.2 发布目标配置相关

- `GET /api/publish/target/list`
- `GET /api/publish/target/detail`
- `POST /api/publish/target/save`
- `POST /api/publish/target/test`
- `DELETE /api/publish/target/remove`

### 11.3 发布执行相关

- `POST /api/publish/start`
- `GET /api/publish/task/status`
- `GET /api/publish/task/stream`

`/api/publish/start` 请求体建议：

```json
{
  "scope": "single_book",
  "books": ["b_the_road_less_traveled"],
  "mode": "incremental",
  "target_type": "s3",
  "target_id": "prod-main"
}
```

门户发布示例：

```json
{
  "scope": "portal_only",
  "books": [],
  "mode": "overwrite",
  "target_type": "local_dir",
  "target_id": "local-main"
}
```

### 11.4 发布记录相关

- `GET /api/publish/record/list`
- `GET /api/publish/record/detail`
- `GET /api/publish/record/files`
- `POST /api/publish/record/restore`

## 12. 实施阶段建议

为了降低改造风险，建议按下面顺序推进。

### 第一阶段：先改前端入口与交互

- 移动索引入口到搜索下方
- 移动发布入口到搜索下方
- 增加统一操作侧栏
- 增加未保存检测 + 二次确认

### 第二阶段：索引逻辑模式化

- 增加“增量补充新索引”
- 增加“全量更新”
- 增加“预览变更”
- 保留现有扫描逻辑作为底层能力

### 第三阶段：发布目标配置解耦

- 保留附件上传使用 `system.yaml` 的系统 S3 配置
- 只把“发布目标配置”从系统配置里拆出
- 引入 `publish_targets` 配置目录
- 增加本地目录配置与 SFTP 配置结构

### 第四阶段：发布器抽象化

- 抽象 `Publisher` 接口
- 落地 `S3Publisher`
- 落地 `LocalDirPublisher`
- 落地 `SFTPPublisher`
- 落地“增量 / 覆盖 + bak 版本目录”流程

### 第五阶段：发布记录增强与恢复能力

- 保存文件 MAP
- 增加记录详情页
- 增加备份路径展示
- 增加“从备份版本恢复”

## 13. 风险与注意点

### 13.1 索引覆盖风险

如果直接上“全量更新”且默认执行，仍然会覆盖人工编辑过的权重和首页展示配置。

因此建议：

- 默认选中“增量补充新索引”
- “全量更新”使用更强确认文案

### 13.2 覆盖模式风险

如果目标目录就是线上站点目录，覆盖模式中的“移动旧目录 -> 发布新目录”期间会存在短暂切换窗口。

建议：

- 在确认弹窗中提示
- 记录完整备份路径
- 后续如有必要，再补“临时目录发布后切换”

### 13.3 SFTP 备份风险

如果远程目录很大，覆盖模式下移动到 `bak` 目录可能耗时较长。

建议：

- 在配置测试阶段校验 `bak` 目录权限
- 在发布确认中显示将要备份的目标目录
- 日志里明确打印备份版本号

### 13.4 记录体积增长

如果保存完整文件 MAP，记录文件会明显变大。

建议方案：

- 保留 YAML 主记录
- 文件映射单独存 `record_xxx.files.yaml` 或 `record_xxx.files.json`

## 14. 我建议优先拍板的几个关键决策

为了后续实现不反复，建议先确认以下决策：

1. 索引默认模式是否采用“增量补充新索引”
2. 发布中心是否统一走“搜索下方入口 + 右侧侧栏”
3. 发布范围是否先固定为“整站 / 书籍 / 门户”
4. 发布目标配置是否采用“独立 YAML 文件目录”
5. 覆盖模式的 `bak` 目录是固定在目标目录下，还是允许单独配置

## 15. 结论

这次改版的核心，不是单纯挪按钮，而是把发布器从“直接执行工具”升级为“有操作上下文的发布中心”。

其中最关键的几件事是：

- 索引从“覆盖式重建”升级为“模式化索引维护”
- 发布从“系统 S3 唯一发布”升级为“多目标发布架构”
- 附件上传继续沿用系统 S3 配置，不和发布配置混用
- 发布模式明确区分“增量发布”和“覆盖发布 + bak 备份”
- 发布记录从“日志记录”升级为“可恢复的发布快照”

如果按这个规划推进，后面不管继续扩充发布类型，还是做更完整的环境区分与恢复流程，都会顺很多。
