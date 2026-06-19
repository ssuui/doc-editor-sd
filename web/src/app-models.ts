export type TreeNode = { name: string; path: string; type: "file" | "folder"; children?: TreeNode[] };
export type BookItem = { book_dir_name: string; weight: number };
export type HomeBookItem = { book_dir_name: string; weight: number; enable_home_show: boolean };
export type TaskInfo = { id: string; status: string; result_url: string; error_msg: string; done: boolean };
export type IndexTarget = "site_meta" | "books_meta";
export type IndexMode = "append_new" | "full_refresh";
export type PublishScope = "full_site" | "single_book" | "portal_only";
export type PublishMode = "incremental" | "overwrite";
export type PublishTarget = {
  id: string;
  name: string;
  type: "s3" | "local_dir" | "sftp";
  enabled: boolean;
  mode_default?: string;
  bucket?: string;
  region?: string;
  endpoint?: string;
  access_key_id?: string;
  secret_access_key?: string;
  site_public_domain?: string;
  base_prefix?: string;
  cache_html?: string;
  cache_static?: string;
  target_dir?: string;
  bak_dir?: string;
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  private_key_path?: string;
  remote_dir?: string;
  remote_bak_dir?: string;
};
export type PublishTargetType = PublishTarget["type"];
export type PublishRecord = {
  record_id: string;
  publishing_time: string;
  publishing_scope: string;
  publish_mode: string;
  publishing_target_type: string;
  publishing_target_id: string;
  publishing_target_name: string;
  build_books: string[];
  public_url: string;
  status: string;
  error_msg: string;
  backup_path: string;
};
export type IndexPlan = {
  targets: string[];
  mode: string;
  site?: { changed: boolean; items: HomeBookItem[]; changes: Array<{ type: string; book_dir_name: string; detail: string }> };
  books?: { changed: boolean; items: BookItem[]; changes: Array<{ type: string; book_dir_name: string; detail: string }> };
};

export function createEmptyPublishTarget(type: PublishTargetType, mode: PublishMode): PublishTarget {
  return {
    id: "",
    name: "",
    type,
    enabled: true,
    mode_default: mode
  };
}

export function publishScopeLabel(scope: PublishScope) {
  switch (scope) {
    case "full_site":
      return "整站";
    case "single_book":
      return "书籍";
    case "portal_only":
      return "门户";
    default:
      return scope;
  }
}

export function publishModeLabel(mode: PublishMode) {
  return mode === "overwrite" ? "覆盖模式" : "增量模式";
}

export function publishTargetTypeLabel(type: PublishTarget["type"]) {
  switch (type) {
    case "s3":
      return "S3";
    case "local_dir":
      return "目录";
    case "sftp":
      return "SSH / SFTP";
    default:
      return type;
  }
}

export function publishScopeDescription(scope: PublishScope) {
  switch (scope) {
    case "full_site":
      return "重新构建门户与所有书籍，再统一发布到目标位置。";
    case "single_book":
      return "只发布当前书籍，适合内容更新后的快速同步。";
    case "portal_only":
      return "只发布门户首页与书单入口，不动各书籍目录。";
    default:
      return "";
  }
}

export function publishModeDescription(mode: PublishMode) {
  return mode === "overwrite"
    ? "发布前先备份旧目录，再写入新文件。"
    : "只覆盖本次涉及的同路径文件，不处理其它历史文件。";
}
