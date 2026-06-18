# 书籍适配规范

本文档用于说明如何把外部电子书资料整理成适配当前项目的书籍目录。后续如果在 `temp/` 或其它位置拿到新的 `epub` 文件，请按本文档处理后再接入 `service/source_root`。

## 目标

- 让书籍能被当前 Hugo 站点正常识别。
- 让侧边栏、首页卡片、书单页都按项目约定工作。
- 让目录路径保持英文，阅读标题保持中文。
- 让后续维护者不需要重新摸索目录和排序规则。

## 最终目录结构

每本书都在 `service/source_root/` 下创建一个独立目录，结构固定如下：

```text
service/source_root/
  b_xxx_book_slug/
    book_meta.yaml
    hugo.toml
    static-img/
    content/
      _index.md
      01-section-slug/
        _index.md
        01-page-slug.md
        02-page-slug.md
```

## 目录命名规则

### 1. 书籍根目录

- 目录名必须使用英文 slug。
- 统一以 `b_` 开头。
- 不要直接使用中文目录名。

示例：

- `b_the_road_less_traveled`
- `b_the_courage_to_be_disliked`

### 2. `content` 下的子目录

- 子目录也使用英文 slug。
- 保留数字前缀，确保自然排序稳定。
- 子目录负责表达“卷 / 页 / 部分 / 前后记”这类阅读层级。

示例：

- `01-maturity-journey`
- `02-people-of-the-lie`
- `03-second-night`

### 3. 页面文件名

- 所有 `.md` 文件名使用英文。
- 保留数字前缀，确保顺序清晰。
- 文件名中不要保留中文。

示例：

- `01-overview.md`
- `04-chapter-1-thinking.md`
- `02-chinese-preface.md`

## 标题显示规则

### 1. 子目录中的 `_index.md`

- 只保留中文标题。
- 不要放说明文字、封面图、摘要、列表。
- 作用是让菜单栏显示中文。

示例：

```md
# 第二夜 一切烦恼都来自人际关系
```

### 2. `content/_index.md`

- 同样只保留书名标题。
- 不要放介绍性内容。

示例：

```md
# 被讨厌的勇气
```

### 3. 正文页面

- 页面正文文件内使用中文标题，便于阅读。
- 文件路径英文、页面标题中文，这两者要分开处理。

## 图片处理规则

### 1. 图片存放

- 书内图片统一放到书籍目录下的 `static-img/`。
- 封面图也放这里。

### 2. 图片引用

- Markdown 中统一写成站内路径：

```md
![封面](/static-img/cover.jpg)
```

- 不再写外部 CDN 全地址。
- `book_meta.yaml` 里的 `cover_img` 也使用 `/static-img/...`。

### 3. 装饰图处理

- 如果图片只是重复出现的装饰元素，要根据阅读效果决定是否保留。
- 当前项目允许保留，但如果影响阅读，建议在导入时批量清理。

## 页面拆分规则

不要把整本书塞进单个页面。

优先按“读者可理解的章节层级”拆分：

- 有明显卷册结构：先拆卷，再拆章。
- 有明显“第一夜 / 第二夜”结构：按夜拆目录，再拆小节。
- 有前言、推荐序、后记、作者简介：单独成页。

经验规则：

- 一个页面尽量只承载一个明确主题。
- 如果原书某一章过长，要继续按原目录中的二级小节拆分。
- 如果原书只有很短的前置信息，可以保留为单页，但不要和正文混在一起。

## `book_meta.yaml` 规范

每本书都必须有 `book_meta.yaml`，至少包含：

```yaml
display_name: "中文书名"
cover_img: "/static-img/cover.jpg"
description: "简短描述"
tags: ["读书", "心理学"]
version: "imported"
visible_in_home: false
sidebar_order:
  - 01-section/_index.md
  - 01-section/01-page.md
  - 01-section/02-page.md
```

重点说明：

- `display_name` 使用中文。
- `cover_img` 使用站内相对路径。
- `sidebar_order` 必须按“文件”排序，不能只写目录名。

## `sidebar_order` 规则

这是当前项目里最容易出错的一项。

### 必须这样做

- 把所有页面按阅读顺序逐个写入 `sidebar_order`。
- 每个子目录的 `_index.md` 也要写进去。
- 不能只写：

```yaml
sidebar_order:
  - 01-section
  - 02-section
```

### 必须写成

```yaml
sidebar_order:
  - 01-section/_index.md
  - 01-section/01-page.md
  - 01-section/02-page.md
  - 02-section/_index.md
  - 02-section/01-page.md
```

### 原因

项目里的侧边栏权重生成逻辑会读取 `sidebar_order`，并按文件路径分配页面顺序。只排目录时，子页面虽然能推断顺序，但不够精确；逐文件写入才能完全锁定菜单顺序。

## 站点索引接入

新增书籍后，需要同步更新两个文件：

- `service/source_root/_books_meta.yaml`
- `service/source_root/_site_meta.yaml`

### `_books_meta.yaml`

- 控制书单整体顺序。

示例：

```yaml
- book_dir_name: b_the_courage_to_be_disliked
  weight: 60
```

### `_site_meta.yaml`

- 控制站点是否在首页展示。

示例：

```yaml
- book_dir_name: b_the_courage_to_be_disliked
  weight: 60
  enable_home_show: false
```

## `hugo.toml` 规范

每本书的根目录下保留最小配置：

```toml
baseURL = "/b_the_courage_to_be_disliked/"
languageCode = "zh-cn"
title = "被讨厌的勇气"
```

要求：

- `baseURL` 与书籍目录名一致。
- `title` 使用中文书名。

## 推荐操作流程

### 1. 准备源文件

- 把电子书放到 `temp/`。
- 确认格式是否为 `epub`。

### 2. 提取内容

- 读取目录结构。
- 识别封面、正文、前言、后记、章节、小节。
- 提取图片资源。

### 3. 建立书籍目录

- 创建 `b_英文slug/`。
- 写入 `hugo.toml`、`book_meta.yaml`。
- 创建 `static-img/` 与 `content/`。

### 4. 生成内容页

- 子目录英文命名。
- `_index.md` 仅保留中文标题。
- 页面文件名英文命名。
- 正文标题保持中文。

### 5. 处理图片

- 所有引用改为 `/static-img/...`。
- `cover_img` 同步改为相对路径。

### 6. 写入 `sidebar_order`

- 严格按页面顺序写完整文件列表。

### 7. 接入全局索引

- 更新 `_books_meta.yaml`
- 更新 `_site_meta.yaml`

### 8. 最后检查

- 路径是否全英文。
- 标题是否全中文。
- `_index.md` 是否只有标题。
- `sidebar_order` 是否按文件逐项列出。
- 图片是否都使用 `/static-img/...`。

## 当前目录中的参考实现

可直接参考以下两个已完成目录：

- `service/source_root/b_the_road_less_traveled`
- `service/source_root/b_the_courage_to_be_disliked`

其中重点参考：

- 英文目录与英文文件名
- 中文 `_index.md`
- `/static-img/...` 图片路径
- `book_meta.yaml` 中逐文件的 `sidebar_order`

## 不要这样做

- 不要使用中文目录名或中文文件名。
- 不要在 `_index.md` 中放大段介绍性内容。
- 不要把整本书压成一个页面。
- 不要在 `book_meta.yaml` 的 `sidebar_order` 中只写目录名。
- 不要继续使用外部完整图片 URL。

## 建议

- 每接入一本新书，都先模仿现有两本已完成的目录再开始。
- 如果书的目录层级特别复杂，优先保证“菜单结构清晰”而不是“一次性完全自动化”。
- 如果出现重复装饰图、空白页、纯目录页，导入时可以主动清理。
