import "reflect-metadata";
import React, { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import { marked } from "marked";
import { KeyCode, KeyMod, type editor as MonacoEditorNS } from "monaco-editor";
import { container } from "tsyringe";
import molecule, { Workbench, create } from "@dtinsight/molecule";
import { MonacoEditor } from "@dtinsight/molecule/esm/components";
import type { IPanelItem } from "@dtinsight/molecule/esm/model/workbench/panel";
import type { IFolderTreeNodeProps } from "@dtinsight/molecule/esm/model/workbench/explorer/folderTree";
import type { IEditorActionsProps } from "@dtinsight/molecule/esm/model/workbench/editor";
import type { IActivityBarItem } from "@dtinsight/molecule/esm/model/workbench/activityBar";
import { BuiltinService } from "@dtinsight/molecule/esm/services";
import { MonacoService } from "@dtinsight/molecule/esm/monaco/monacoService";
import { CommandQuickAccessViewAction } from "@dtinsight/molecule/esm/monaco/quickAccessViewAction";
import { Float } from "@dtinsight/molecule/esm/model/workbench/statusBar";
import { NotificationStatus } from "@dtinsight/molecule/esm/model/notification";
import "@dtinsight/molecule/esm/style/mo.css";
import { request } from "./request/client";
import "./styles.css";

type TreeNode = { name: string; path: string; type: "file" | "folder"; children?: TreeNode[] };
type BookItem = { book_dir_name: string; weight: number };
type HomeBookItem = { book_dir_name: string; weight: number; enable_home_show: boolean };
type TaskInfo = { id: string; status: string; result_url: string; error_msg: string; done: boolean };

type DialogState =
  | {
      type: "entry";
      mode: "file" | "folder" | "rename";
      title: string;
      label: string;
      confirmText: string;
      value: string;
      placeholder?: string;
      basePath?: string;
      targetPath?: string;
    }
  | {
      type: "delete";
      title: string;
      message: string;
      confirmText: string;
      path: string;
    }
  | {
      type: "action";
      title: string;
      message?: string;
      actions: Array<{
        id: string;
        label: string;
      }>;
    };

type AppState = {
  currentPath: string;
  currentContent: string;
  currentBook: string;
  logs: string;
  publishState: string;
  publishURL: string;
  publishBusy: boolean;
  publishStartedAt: number;
  publishLastEventAt: number;
  treeContextPath: string;
  treeContextType: "file" | "folder" | "";
  dialog: DialogState | null;
  dialogBusy: boolean;
};

function createStore(initial: AppState) {
  let state = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => state,
    set: (patch: Partial<AppState>) => {
      state = { ...state, ...patch };
      listeners.forEach((listener) => listener());
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }
  };
}

const store = createStore({
  currentPath: "",
  currentContent: "",
  currentBook: "",
  logs: "",
  publishState: "空闲",
  publishURL: "",
  publishBusy: false,
  publishStartedAt: 0,
  publishLastEventAt: 0,
  treeContextPath: "",
  treeContextType: "",
  dialog: null,
  dialogBusy: false
});

marked.setOptions({ gfm: true, breaks: true });

const FOLDER_PANEL_ID = "sidebar.explore.folders";
const EDITOR_UPLOAD_ACTION_ID = "editor.upload-asset";
const EXPLORER_NEW_FILE_ID = "custom.new-file";
const EXPLORER_NEW_FOLDER_ID = "custom.new-folder";
const EXPLORER_REFRESH_ID = "custom.refresh";
const EXPLORER_UPLOAD_ID = "custom.upload";
const GLOBAL_LOGOUT_ID = "global.menu.logout";
const GLOBAL_SETTINGS_ID = "global.menu.settings.custom";
const ACTIVITY_TOGGLE_PANEL_ID = "cms.activity.togglePanel";
const ACTIVITY_REBUILD_HOME_META_ID = "cms.activity.rebuildHomeMeta";
const ACTIVITY_REBUILD_BOOKS_META_ID = "cms.activity.rebuildBooksMeta";
const ACTION_QUICK_COMMAND = "editor.action.quickCommand";
const ACTION_QUICK_ACCESS_SETTINGS = "workbench.action.quickAccessSettings";
const ACTION_SELECT_THEME = "workbench.action.selectTheme";
const ACTION_THEME_DIALOG = "cms.action.themeDialog";
const ACTION_SEARCH_HELP = "cms.action.searchHelp";
const WORKBENCH_SETTINGS_PATH = "__cms__/workbench.settings.json";

const customLocaleExtension = {
  id: "CmsCustomLocales",
  name: "Cms Custom Locales",
  contributes: {
    languages: [{
      id: "zh-CN-cms",
      name: "简体中文",
      inherit: "zh-CN",
      source: {
        "themes.selectTheme": "选择主题颜色（上下键预览）",
        "selectTheme.label": "主题颜色",
        "showTriggerActions": "命令面板",
        "locale.select": "选择显示语言（上下键预览）",
        "select.locale": "选择显示语言",
        "menu.colorTheme": "主题颜色",
        "quickAccessSettings.label": "打开设置（JSON）",
        "menu.settings": "设置",
        "search.matchCase": "区分大小写",
        "search.matchWholeWord": "全词匹配",
        "search.useRegularExpression": "使用正则表达式",
        "search.replaceAll": "全部替换",
        "sidebar.replace.placement": "替换"
      }
    }]
  },
  activate() {},
  dispose() {}
};

container.resolve(BuiltinService).inactiveModule("builtInExplorerOutlinePanel");
const mo = create({ extensions: [customLocaleExtension], defaultLocale: "zh-CN-cms" });
let bootstrapped = false;
let activeEditorInstance: MonacoEditorNS.IStandaloneCodeEditor | null = null;
let activeEditorPath = "";
let activeTabPath = "";
let activeTabGroupId: string | number | undefined;
let currentTree: TreeNode[] = [];
const fileContentCache = new Map<string, string>();
const documentStateByPath = new Map<string, {
  content: string;
  modified: boolean;
  groupId?: string | number;
}>();
const editorInstanceByPath = new Map<string, MonacoEditorNS.IStandaloneCodeEditor>();
let currentExpandKeys: string[] = ["source_root"];
let explorerContextTarget: { path: string; type?: "file" | "folder" } | null = null;
let closeGuardBypassDepth = 0;
let closeGuardsInstalled = false;

function useAppState() {
  return useSyncExternalStore(store.subscribe, store.get, store.get);
}

function updateDocumentState(path: string, patch: {
  content?: string;
  modified?: boolean;
  groupId?: string | number;
}) {
  if (!path) return null;
  const prev = documentStateByPath.get(path) || { content: fileContentCache.get(path) || "", modified: false };
  const next = {
    content: patch.content ?? prev.content,
    modified: patch.modified ?? prev.modified,
    groupId: patch.groupId ?? prev.groupId
  };
  documentStateByPath.set(path, next);
  if (typeof patch.content === "string") {
    fileContentCache.set(path, patch.content);
  }
  if (activeTabPath === path || store.get().currentPath === path) {
    store.set({ currentPath: path, currentContent: next.content });
  }
  return next;
}

function getDocumentState(path: string) {
  return path ? documentStateByPath.get(path) || null : null;
}

function PublishPane() {
  const state = useAppState();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    container.scrollTop = container.scrollHeight;
  }, [state.logs, state.publishState]);

  useEffect(() => {
    if (!state.publishBusy) return;
    const timer = window.setInterval(() => {
      setTick((value) => value + 1);
    }, 240);
    return () => window.clearInterval(timer);
  }, [state.publishBusy]);

  const now = Date.now();
  const elapsedMs = state.publishStartedAt ? Math.max(0, now - state.publishStartedAt) : 0;
  const idleMs = state.publishLastEventAt ? Math.max(0, now - state.publishLastEventAt) : 0;
  const spinnerFrames = ["|", "/", "-", "\\"];
  const spinner = spinnerFrames[tick % spinnerFrames.length];
  const elapsedLabel = formatDuration(elapsedMs);
  const idleLabel = formatDuration(idleMs);

  return (
    <div ref={containerRef} className="custom-pane">
      <div className="pane-title">发布日志</div>
      <div className="pane-meta">状态：{state.publishState}</div>
      {state.publishBusy ? (
        <div className="pane-activity" aria-live="polite">
          <div className="pane-activity-row">
            <span className="pane-activity-spinner" aria-hidden="true">{spinner}</span>
            <span>任务执行中，已等待 {elapsedLabel}</span>
            <span className="pane-activity-idle">距上次日志 {idleLabel}</span>
          </div>
          <div className="pane-activity-bar" aria-hidden="true">
            <span className="pane-activity-bar-inner" />
          </div>
        </div>
      ) : null}
      {state.publishURL ? <div className="pane-meta">访问地址：<a href={state.publishURL} target="_blank" rel="noreferrer">{state.publishURL}</a></div> : null}
      <pre className="pane-log">{state.logs || "暂无日志"}</pre>
    </div>
  );
}

function formatDuration(ms: number) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes > 0) {
    return `${minutes}分${seconds.toString().padStart(2, "0")}秒`;
  }
  return `${seconds}秒`;
}

function DialogLayer() {
  const state = useAppState();
  const dialog = state.dialog;
  const [value, setValue] = useState("");

  useEffect(() => {
    if (dialog?.type === "entry") {
      setValue(dialog.value);
      return;
    }
    setValue("");
  }, [dialog]);

  if (!dialog) return null;

  const close = () => {
    if (state.dialogBusy) return;
    store.set({ dialog: null });
  };

  const submit = async () => {
    if (state.dialogBusy) return;
    store.set({ dialogBusy: true });
    try {
      if (dialog.type === "entry") {
        await submitEntryDialog(dialog, value);
      } else if (dialog.type === "action") {
        return;
      } else {
        await submitDeleteDialog(dialog.path);
      }
      store.set({ dialog: null });
    } catch (error) {
      notifyError(error instanceof Error ? error.message : "操作失败");
    } finally {
      store.set({ dialogBusy: false });
    }
  };

  return (
    <div className="app-modal-backdrop" onClick={close}>
      <div
        className="app-modal"
        role="dialog"
        aria-modal="true"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="app-modal-header">
          <h3>{dialog.title}</h3>
          <button type="button" className="app-modal-close" onClick={close}>关闭</button>
        </div>
        {dialog.type === "entry" ? (
          <div className="app-modal-body">
            <label className="app-modal-label">
              <span>{dialog.label}</span>
              <input
                className="app-modal-input"
                value={value}
                placeholder={dialog.placeholder}
                onChange={(event) => setValue(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void submit();
                  if (event.key === "Escape") close();
                }}
                autoFocus
              />
            </label>
          </div>
        ) : dialog.type === "delete" ? (
          <div className="app-modal-body">
            <p className="app-modal-message">{dialog.message}</p>
          </div>
        ) : (
          <div className="app-modal-body">
            {dialog.message ? <p className="app-modal-message">{dialog.message}</p> : null}
            <div className="app-action-list">
              {dialog.actions.map((action) => (
                <button
                  key={action.id}
                  type="button"
                  className="app-action-item"
                  onClick={() => {
                    void handleActionDialog(action.id);
                  }}
                >
                  {action.label}
                </button>
              ))}
            </div>
          </div>
        )}
        <div className="app-modal-actions">
          <button type="button" className="app-button app-button-secondary" onClick={close} disabled={state.dialogBusy}>取消</button>
          {dialog.type === "action" ? null : (
            <button type="button" className="app-button app-button-primary" onClick={() => void submit()} disabled={state.dialogBusy}>
              {state.dialogBusy ? "处理中..." : dialog.confirmText}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function MarkdownSplitPane({
  path,
  value,
  groupId
}: {
  path: string;
  value: string;
  groupId?: string | number;
}) {
  const editorRef = useRef<MonacoEditorNS.IStandaloneCodeEditor | null>(null);
  const changeSubscriptionRef = useRef<{ dispose: () => void } | null>(null);
  const focusSubscriptionRef = useRef<{ dispose: () => void } | null>(null);
  const syncingRef = useRef(false);
  const [previewHTML, setPreviewHTML] = useState(() => renderMarkdown(value));

  const rebindEditor = (editor: MonacoEditorNS.IStandaloneCodeEditor) => {
    changeSubscriptionRef.current?.dispose();
    focusSubscriptionRef.current?.dispose();
    editorRef.current = editor;
    bindEditorToPath(path, editor);
    setActiveEditor(path, editor);
    attachCustomEditorContextMenu(editor);
    registerEditorCommands(editor);

    changeSubscriptionRef.current = editor.onDidChangeModelContent(() => {
      if (syncingRef.current) return;
      const nextValue = editor.getValue();
      updateDocumentState(path, { content: nextValue, modified: true, groupId });
      setActiveTab(path, groupId, nextValue);
      markTabEdited(path, groupId);
      setPreviewHTML(renderMarkdown(nextValue));
    });

    focusSubscriptionRef.current = editor.onDidFocusEditorText(() => {
      bindEditorToPath(path, editor);
      setActiveTab(path, groupId, editor.getValue());
      setActiveEditor(path, editor);
    });
  };

  // 切换文件(path 变化)时,用新内容覆盖编辑器。注意 editor.setValue 会清空
  // 撤销栈,所以不能在 value 变化时执行 —— 否则用户每输入一个字符,父组件
  // 更新 tab.data.value 回传进来,undo 栈就被清空,Ctrl+Z 直接失效。
  // 因此这里依赖只放 path,内容由编辑器自身 + onDidChangeModelContent 维护。
  useEffect(() => {
    setPreviewHTML(renderMarkdown(value));
    const editor = editorRef.current;
    if (!editor) return;
    if (editor.getValue() === value) return;
    syncingRef.current = true;
    editor.setValue(value);
    syncingRef.current = false;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    rebindEditor(editor);
    // 这里要在 path/groupId 变化时强制重绑。Molecule 在多 tab 间可能复用
    // 同一个 Monaco 实例，如果不重绑，监听器里的 path 会停留在旧文件。
  }, [path, groupId]);

  useEffect(() => {
    return () => {
      changeSubscriptionRef.current?.dispose();
      focusSubscriptionRef.current?.dispose();
      const boundEditor = editorInstanceByPath.get(path);
      if (boundEditor && boundEditor === editorRef.current) {
        editorInstanceByPath.delete(path);
      }
      if (activeEditorPath === path) {
        activeEditorInstance = null;
        activeEditorPath = "";
      }
    };
  }, [path]);

  const bindEditor = (editor: MonacoEditorNS.IStandaloneCodeEditor) => {
    rebindEditor(editor);
  };

  return (
    <div className="markdown-workspace">
      <div className="markdown-editor-pane">
        <MonacoEditor
          path={path}
          options={{
            ...molecule.editor.getState().editorOptions,
            value,
            language: "markdown",
            automaticLayout: true,
            contextmenu: false
          }}
          editorInstanceRef={bindEditor}
        />
      </div>
      <div className="markdown-preview-pane">
        <div className="markdown-preview-header">Markdown 预览</div>
        <div className="markdown-body" dangerouslySetInnerHTML={{ __html: previewHTML }} />
      </div>
    </div>
  );
}

function App() {
  useEffect(() => {
    if (bootstrapped) return;
    bootstrapped = true;
    void setupWorkbench();
  }, []);

  return mo.render(
    <>
      <Workbench />
      <DialogLayer />
    </>
  );
}

function ImagePreviewPane({ path }: { path: string }) {
  return (
    <div className="image-preview-pane">
      <div className="image-preview-stage">
        <img className="image-preview-content" src={buildRawFileURL(path)} alt={basename(path)} />
      </div>
    </div>
  );
}

function ReadonlyTextPane({ path, value, language = "plaintext" }: { path: string; value: string; language?: string }) {
  return (
    <MonacoEditor
      path={path}
      options={{
        ...molecule.editor.getState().editorOptions,
        value,
        language,
        automaticLayout: true,
        readOnly: true,
        domReadOnly: true,
        contextmenu: false,
        minimap: { enabled: false }
      }}
    />
  );
}

function PdfPreviewPane({ path }: { path: string }) {
  const src = buildRawFileURL(path);
  return (
    <div className="pdf-preview-pane">
      <object
        className="pdf-preview-frame"
        data={src}
        type="application/pdf"
        aria-label={basename(path)}
      >
        <iframe
          className="pdf-preview-frame"
          src={src}
          title={basename(path)}
        />
        <div className="pdf-preview-fallback">
          <p>当前浏览器内嵌 PDF 预览不可用。</p>
          <a href={src} target="_blank" rel="noreferrer">打开 PDF</a>
        </div>
      </object>
    </div>
  );
}

async function setupWorkbench() {
  molecule.layout.setPaneSize(["22%", "78%"]);
  molecule.layout.setHorizontalPaneSize(["68%", "32%"]);
  if (!molecule.layout.getState().menuBar.hidden) {
    molecule.layout.toggleMenuBarVisibility();
  }
  molecule.editor.updateEditorOptions({
    fontSize: 14,
    wordWrap: "on",
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    contextmenu: false
  });
  installUnsavedCloseGuards();

  setupPanels();
  setupFolderTree();
  setupExplorerToolbar();
  setupStatusBar();
  setupActivityBar();
  setupEditorIntegration();
  setupSearchIntegration();
  setupThemePersistence();
  setupUiLocalization();
  setupOverflowImprovements();
  await refreshAll();
}

function setupPanels() {
  const panels: IPanelItem[] = [
    { id: "publish", name: "发布日志", closable: false, renderPane: () => <PublishPane /> }
  ];
  molecule.panel.add(panels);
  molecule.panel.open(panels[0]);
  // 移除侧边栏 Explorer 里无实际功能的 "大纲(OUTLINE)" 面板。
  // 该面板由 Molecule 内置 OutlineController.initView 异步注入,这里重试几次
  // 确保在面板就绪后移除,避免初始化时序导致移除失败。
  const removeOutline = (attempt = 0) => {
    const panels = molecule.explorer.getState()?.panels || [];
    const exists = panels.some((p: { id: unknown }) => String(p.id) === "outline");
    if (exists) {
      molecule.explorer.removePanel("outline");
      return;
    }
    if (attempt < 80) window.setTimeout(() => removeOutline(attempt + 1), 180);
  };
  removeOutline();
  const observer = new MutationObserver(() => removeOutline());
  observer.observe(document.body, { childList: true, subtree: true });
}

function setupExplorerToolbar() {
  molecule.explorer.updatePanel({
    id: FOLDER_PANEL_ID,
    name: "站点文件",
    toolbar: [
      { id: EXPLORER_NEW_FILE_ID, title: "新建文件", icon: "new-file" },
      { id: EXPLORER_NEW_FOLDER_ID, title: "新建文件夹", icon: "new-folder" },
      { id: EXPLORER_UPLOAD_ID, title: "上传附件", icon: "cloud-upload" },
      { id: EXPLORER_REFRESH_ID, title: "刷新", icon: "refresh" }
    ]
  });

  molecule.explorer.onPanelToolbarClick((panel, toolbarId) => {
    if (panel.id !== FOLDER_PANEL_ID) return;
    if (toolbarId === EXPLORER_NEW_FILE_ID) {
      const target = getPreferredCreationTarget();
      openCreateDialog("file", target.path, target.type);
    }
    if (toolbarId === EXPLORER_NEW_FOLDER_ID) {
      const target = getPreferredCreationTarget();
      openCreateDialog("folder", target.path, target.type);
    }
    if (toolbarId === EXPLORER_UPLOAD_ID) {
      void triggerUploadToServer();
    }
    if (toolbarId === EXPLORER_REFRESH_ID) {
      void refreshAll();
    }
  });
}

function setupStatusBar() {
  molecule.statusBar.reset();
  molecule.statusBar.add({
    id: "book",
    name: "未选书籍",
    render: () => <span>{store.get().currentBook || "未选书籍"}</span>
  }, Float.left);
  molecule.statusBar.add({
    id: "full-publish",
    name: "完整发布",
    onClick: () => void startPublish("full")
  }, Float.right);
  molecule.statusBar.add({
    id: "single-publish",
    name: "发布当前书籍",
    onClick: () => void startPublish("single")
  }, Float.right);
}

function setupFolderTree() {
  // 文件列表面板的空白区域右键,也弹出 IDE 风格菜单(新建/上传/刷新),
  // 不再回退到浏览器默认菜单。仅绑定在 Explorer 面板容器上,不影响其它区域。
  bindExplorerContextMenu();
  bindExplorerSelectionSync();
  molecule.folderTree.onExpandKeys((expandKeys) => {
    currentExpandKeys = expandKeys.map(String);
    molecule.folderTree.setExpandKeys(expandKeys);
  });
  molecule.folderTree.onSelectFile((file) => {
    if (file.location) {
      molecule.folderTree.setActive(file.location);
      explorerContextTarget = { path: file.location, type: "file" };
      store.set({ treeContextPath: file.location, treeContextType: "file" });
      void openFile(file.location);
    }
  });
  molecule.folderTree.onRightClick((node) => {
    if (!node?.location) return;
    syncBook(node.location);
    molecule.folderTree.setActive(node.location);
    explorerContextTarget = {
      path: node.location,
      type: node.fileType === "Folder" || node.fileType === "RootFolder" ? "folder" : "file"
    };
    store.set({
      treeContextPath: node.location,
      treeContextType: node.fileType === "Folder" || node.fileType === "RootFolder" ? "folder" : "file"
    });
  });
  molecule.folderTree.onDropTree((source, target) => {
    void moveTreeEntry(source, target);
  });
}

function setupActivityBar() {
  molecule.activityBar.remove(["global.menu.account", "global.menu.settings"]);

  const customItems: IActivityBarItem[] = [
    {
      id: ACTIVITY_TOGGLE_PANEL_ID,
      name: "底栏",
      title: "显示或隐藏底栏",
      icon: "list-tree",
      type: "global"
    },
    {
      id: ACTIVITY_REBUILD_HOME_META_ID,
      name: "首页推荐",
      title: "扫描书籍目录并重建 _site_meta.yaml",
      icon: "repo",
      type: "global"
    },
    {
      id: ACTIVITY_REBUILD_BOOKS_META_ID,
      name: "书籍列表",
      title: "扫描书籍目录并重建 _books_meta.yaml",
      icon: "library",
      type: "global"
    },
    {
      id: GLOBAL_LOGOUT_ID,
      name: "退出登录",
      title: "退出登录",
      icon: "sign-out",
      type: "global"
    },
    {
      id: GLOBAL_SETTINGS_ID,
      name: "工作台设置",
      title: "工作台设置",
      icon: "settings-gear",
      type: "global"
    }
  ];

  molecule.activityBar.add(customItems);
  molecule.activityBar.onClick((selectedKey) => {
    if (selectedKey === ACTIVITY_TOGGLE_PANEL_ID) {
      toggleBottomPanel();
    }
    if (selectedKey === ACTIVITY_REBUILD_HOME_META_ID) {
      void rebuildSiteMeta();
    }
    if (selectedKey === ACTIVITY_REBUILD_BOOKS_META_ID) {
      void rebuildBooksMeta();
    }
    if (selectedKey === GLOBAL_LOGOUT_ID) {
      openLogoutDialog();
    }
    if (selectedKey === GLOBAL_SETTINGS_ID) {
      openWorkbenchActionDialog();
    }
  });
}

function setupEditorIntegration() {
  const editorActions: IEditorActionsProps[] = [
    {
      id: EDITOR_UPLOAD_ACTION_ID,
      title: "上传附件",
      icon: "cloud-upload",
      place: "outer"
    }
  ];

  molecule.editor.setDefaultActions(editorActions);
  molecule.editor.onActionsClick((actionId) => {
    if (actionId === EDITOR_UPLOAD_ACTION_ID) {
      void triggerUploadImage();
    }
  });

  molecule.editor.onEditorInstanceMount((editor) => {
    attachCustomEditorContextMenu(editor);
    registerEditorCommands(editor);
    editor.addAction({
      id: "cms.upload.asset",
      label: "上传附件",
      contextMenuGroupId: "2_files",
      contextMenuOrder: 1.5,
      run: () => triggerUploadImage()
    });
    editor.onDidFocusEditorText(() => {
      const currentPath = activeEditorPath || getActiveTabPath();
      setActiveTab(currentPath, activeTabGroupId);
      bindEditorToPath(currentPath, editor);
      setActiveEditor(currentPath, editor);
    });
    // 非 Markdown 文件的内容变更:标记 edited。Markdown 由 MarkdownSplitPane
    // 自行管理 edited 状态,两边不能同时写 tab.status,否则会互相覆盖。
    editor.onDidChangeModelContent(() => {
      const currentPath = getActiveTabPath();
      if (!currentPath || currentPath.endsWith(".md")) return;
      const value = editor.getValue();
      bindEditorToPath(currentPath, editor);
      updateDocumentState(currentPath, { content: value, modified: true, groupId: activeTabGroupId });
      setActiveTab(currentPath, activeTabGroupId, value);
      setActiveEditor(currentPath, editor);
      syncEditorTabState(currentPath, { value, modified: true }, activeTabGroupId);
    });
  });

  molecule.editor.onSelectTab((tabId, groupId) => {
    if (groupId === undefined) return;
    const tab = molecule.editor.getTabById(tabId, groupId);
    const path = tab?.data?.path || "";
    setActiveTab(path, groupId, tab?.data?.value || "");
    syncBook(path);
    if (!path.endsWith(".md")) {
      window.setTimeout(() => {
        const editor = molecule.editor.editorInstance;
        if (editor) {
          setActiveTab(path, groupId);
          bindEditorToPath(path, editor);
          setActiveEditor(path, editor);
        }
      }, 0);
    }
  });

  window.addEventListener("keydown", (event) => {
    if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "s") return;
    event.preventDefault();
    void saveCurrentFile();
  }, { capture: true });

  window.addEventListener("keydown", (event) => {
    if (!(event.ctrlKey || event.metaKey) || event.shiftKey !== true || event.key.toLowerCase() !== "p") return;
    event.preventDefault();
    openCommandPaletteNotice();
  }, { capture: true });
}

function getActiveTabSnapshot() {
  const path = activeTabPath || store.get().currentPath;
  const tracked = path ? findEditorTabByPath(path, activeTabGroupId) : null;
  const current = molecule.editor.getState().current;
  const currentTab = tracked?.tab || current?.tab;
  const docState = getDocumentState(path);
  return {
    groupId: tracked?.groupId ?? docState?.groupId ?? activeTabGroupId ?? current?.id,
    tab: currentTab,
    path: currentTab?.data?.path || path,
    value: docState?.content ?? (typeof currentTab?.data?.value === "string" ? currentTab.data.value : undefined)
  };
}

function setupSearchIntegration() {
  const searchPane = molecule.sidebar.get("sidebar.search");
  if (searchPane) {
    molecule.sidebar.update({ ...searchPane, title: "搜索" });
  }
  molecule.search.onChange((value) => {
    if (!value.trim()) {
      molecule.search.setResult([]);
      molecule.search.setValidateInfo("");
    }
  });

  molecule.search.onSearch((value) => {
    void runWorkspaceSearch(value);
  });

  molecule.search.onResultClick((item) => {
    const target = item.data as SearchLeafData | undefined;
    if (!target?.path) return;
    void openFile(target.path, {
      lineNumber: target.lineNumber,
      column: target.column,
      matchLength: target.matchLength
    });
  });
}

function setupThemePersistence() {
  const savedThemeId = localStorage.getItem("cms.workbench.theme");
  if (savedThemeId) {
    const theme = molecule.colorTheme.getThemeById(savedThemeId);
    if (theme) {
      molecule.colorTheme.setTheme(savedThemeId);
    }
  }
  molecule.colorTheme.onChange((_, next) => {
    localStorage.setItem("cms.workbench.theme", next.id);
  });
}

function toggleBottomPanel() {
  const hidden = molecule.layout.togglePanelVisibility();
  molecule.notification.add([{
    id: `panel-${Date.now()}`,
    value: hidden ? "底栏已隐藏" : "底栏已显示",
    status: NotificationStatus.WaitRead
  }]);
}

async function rebuildSiteMeta() {
  const books = await request<HomeBookItem[]>("/api/fs/site/rebuild-meta", { method: "POST" });
  const labels = books.map((book) => `${book.book_dir_name}(${book.enable_home_show ? "首页显示" : "仅目录"})`);
  molecule.notification.add([{
    id: `rebuild-meta-${Date.now()}`,
    value: labels.length ? `已重建 _site_meta.yaml：${labels.join("，")}` : "已重建 _site_meta.yaml，但当前未扫描到书籍目录",
    status: NotificationStatus.WaitRead
  }]);
  await refreshAll();
}

async function rebuildBooksMeta() {
  const books = await request<BookItem[]>("/api/fs/site/rebuild-books-meta", { method: "POST" });
  const labels = books.map((book) => `${book.book_dir_name}(排序 ${book.weight})`);
  molecule.notification.add([{
    id: `rebuild-books-meta-${Date.now()}`,
    value: labels.length ? `已重建 _books_meta.yaml：${labels.join("，")}` : "已重建 _books_meta.yaml，但当前未扫描到书籍目录",
    status: NotificationStatus.WaitRead
  }]);
  await refreshAll();
}

async function refreshAll() {
  const expandedKeys = currentExpandKeys.length ? currentExpandKeys : molecule.folderTree.getExpandKeys().map(String);
  const [siteTree, books] = await Promise.all([
    request<TreeNode[]>("/api/fs/site/root-tree"),
    request<BookItem[]>("/api/fs/book/list")
  ]);

  currentTree = siteTree;
  renderTree(siteTree, expandedKeys, [store.get().treeContextPath, store.get().currentPath]);
  if (!store.get().currentBook && books[0]) {
    syncBook(books[0].book_dir_name);
  }
}

function renderTree(tree: TreeNode[], expandedKeys: Array<string | number> = [], forceExpandPaths: string[] = []) {
  molecule.folderTree.reset();
  const root: IFolderTreeNodeProps = {
    id: "source_root",
    name: "source_root",
    fileType: "RootFolder",
    isLeaf: false,
    children: tree.map(toFolderNode)
  };
  molecule.folderTree.add(root);
  const nextExpanded = Array.from(new Set([
    "source_root",
    ...expandedKeys.map(String),
    ...forceExpandPaths.flatMap(getAncestorPaths)
  ].filter(Boolean)));
  currentExpandKeys = nextExpanded;
  molecule.folderTree.setExpandKeys(nextExpanded);
  if (store.get().currentPath) {
    molecule.folderTree.setActive(store.get().currentPath);
  }
}

function toFolderNode(node: TreeNode): IFolderTreeNodeProps {
  const isFile = node.type === "file";
  return {
    id: node.path,
    name: node.name,
    location: node.path,
    fileType: isFile ? "File" : "Folder",
    isLeaf: isFile,
    children: node.children?.map(toFolderNode)
  };
}

type OpenFileOptions = {
  lineNumber?: number;
  column?: number;
  matchLength?: number;
};

type SearchLeafData = {
  path: string;
  lineNumber: number;
  column: number;
  matchLength: number;
};

async function openFile(path: string, location?: OpenFileOptions) {
  const existing = findEditorTabByPath(path);
  if (existing) {
    const existingState = getDocumentState(path);
    const existingContent = existingState?.content
      ?? (typeof existing.tab.data?.value === "string" ? existing.tab.data.value : fileContentCache.get(path) ?? "");
    syncBook(path);
    store.set({
      currentPath: path,
      currentContent: existingContent,
      treeContextPath: path,
      treeContextType: "file"
    });
    updateDocumentState(path, { content: existingContent, groupId: existing.groupId, modified: existingState?.modified ?? Boolean(existing.tab.data?.modified) });
    setActiveTab(path, existing.groupId, existingContent);
    molecule.editor.setActive(existing.groupId, path);
    if (location?.lineNumber && isEditablePath(path)) {
      focusEditorLocation(path, location);
    }
    return;
  }
  const data = isPreviewablePath(path)
    ? { content: "" }
    : await request<{ content: string }>(`/api/fs/file/content?path=${encodeURIComponent(path)}`);
  if (!isPreviewablePath(path)) {
    fileContentCache.set(path, data.content);
  }
  updateDocumentState(path, { content: data.content, modified: false });
  syncBook(path);
  store.set({
    currentPath: path,
    currentContent: data.content,
    treeContextPath: path,
    treeContextType: "file"
  });
  const groupId = molecule.editor.getGroupIdByTab(path);
  const alreadyOpen = groupId !== null ? molecule.editor.getTabById(path, groupId) : undefined;
  const nextTab = buildEditorTab(path, data.content);
  if (alreadyOpen && groupId !== null) {
    const content = getDocumentState(path)?.content ?? (typeof alreadyOpen.data?.value === "string" ? alreadyOpen.data.value : data.content);
    updateDocumentState(path, { content, groupId, modified: getDocumentState(path)?.modified ?? false });
    setActiveTab(path, groupId, content);
    molecule.editor.setActive(groupId, path);
    molecule.editor.updateTab({
      ...alreadyOpen,
      ...nextTab,
      name: getEditorTabName(path),
      status: getDocumentState(path)?.modified ? "edited" : undefined,
      data: {
        ...(alreadyOpen.data || {}),
        ...(nextTab.data || {}),
        value: content,
        modified: getDocumentState(path)?.modified ?? false
      }
    }, groupId);
    if (location?.lineNumber && isEditablePath(path)) {
      focusEditorLocation(path, location);
    }
    return;
  }
  molecule.editor.open(nextTab);
  const openedGroupId = molecule.editor.getGroupIdByTab(path) ?? undefined;
  updateDocumentState(path, { content: data.content, modified: false, groupId: openedGroupId });
  setActiveTab(path, openedGroupId, data.content);
  if (location?.lineNumber && isEditablePath(path)) {
    focusEditorLocation(path, location);
  }
}

async function saveCurrentFile() {
  const activeTab = getActiveTabSnapshot();
  const currentPath = activeTab.path;
  if (!currentPath) return;
  if (!isEditablePath(currentPath) && currentPath !== WORKBENCH_SETTINGS_PATH) return;
  const content = getCurrentEditorValue(currentPath);
  console.debug("[cms-save]", {
    path: currentPath,
    groupId: activeTab.groupId,
    contentLength: content.length,
    cachedLength: (fileContentCache.get(currentPath) || "").length,
    hasEditorInstance: editorInstanceByPath.has(currentPath),
    activeEditorPath
  });
  if (currentPath === WORKBENCH_SETTINGS_PATH) {
    applyWorkbenchSettings(content);
    markCurrentTabSaved(store.get().currentContent);
    return;
  }
  await request("/api/fs/file/save", {
    method: "PUT",
    body: JSON.stringify({ path: currentPath, content })
  });
  markCurrentTabSaved(content);
  molecule.statusBar.update({
    id: "book",
    name: store.get().currentBook || "未选书籍",
    render: () => <span>{store.get().currentBook || "未选书籍"} 已保存</span>
  }, Float.left);
  window.setTimeout(() => {
    molecule.statusBar.update({
      id: "book",
      name: store.get().currentBook || "未选书籍",
      render: () => <span>{store.get().currentBook || "未选书籍"}</span>
    }, Float.left);
  }, 1600);
}

// 触发编辑器内上传：文件上传到 S3 云存储,并插入 CDN URL 到编辑器
async function triggerUploadImage() {
  const currentBook = store.get().currentBook;
  if (!currentBook) {
    notifyError("请先打开一本书中的 Markdown 文件");
    return;
  }
  const input = document.createElement("input");
  input.type = "file";
  input.accept = "*/*";
  input.onchange = () => {
    const file = input.files?.[0];
    if (!file) return;
    void uploadImage(file).catch((error: unknown) => {
      notifyError(error instanceof Error ? error.message : "附件上传失败");
    });
  };
  input.click();
}

// 触发侧栏上传：文件上传到服务器源码目录,上传后自动刷新文件树
async function triggerUploadToServer() {
  const target = getPreferredCreationTarget();
  const uploadDir = resolveCreationBase(target.path, target.type);
  if (!uploadDir) {
    notifyError("请先在左侧选择一个文件夹");
    return;
  }
  const input = document.createElement("input");
  input.type = "file";
  input.accept = "*/*";
  input.multiple = true;
  input.onchange = async () => {
    const files = Array.from(input.files || []);
    if (!files.length) return;
    let success = 0;
    let fail = 0;
    const uploadedPaths: string[] = [];
    const renamedEntries: Array<{ sourceName: string; finalName: string }> = [];
    for (const file of files) {
      try {
        const result = await uploadFileToServer(uploadDir, file);
        success++;
        uploadedPaths.push(result.path);
        const finalName = basename(result.path);
        if (finalName && finalName !== file.name) {
          renamedEntries.push({ sourceName: file.name, finalName });
        }
      } catch (error: unknown) {
        fail++;
        notifyError(error instanceof Error ? error.message : "上传失败");
      }
    }
    const focusPath = uploadedPaths[uploadedPaths.length - 1] || uploadDir;
    if (focusPath && uploadedPaths.length) {
      store.set({
        treeContextPath: focusPath,
        treeContextType: "file"
      });
    }
    molecule.notification.add([{
      id: `upload-${Date.now()}`,
      value: fail > 0
        ? `上传完成：成功 ${success} 个，失败 ${fail} 个`
        : `已上传 ${success} 个文件到 ${uploadDir}`,
      status: NotificationStatus.WaitRead
    }]);
    if (renamedEntries.length) {
      molecule.notification.add(renamedEntries.map((entry, index) => ({
        id: `upload-rename-${Date.now()}-${index}`,
        value: `同名文件已改名：${entry.sourceName} -> ${entry.finalName}`,
        status: NotificationStatus.WaitRead
      })));
    }
    await refreshAllWithHints([
      ...getAncestorPaths(uploadDir),
      uploadDir,
      ...uploadedPaths
    ], focusPath);
    if (uploadedPaths.length) {
      molecule.folderTree.setActive(focusPath);
      if (isEditablePath(focusPath) || isPreviewablePath(focusPath)) {
        await openFile(focusPath);
      }
    }
  };
  input.click();
}

function validateUpload(file: File) {
  const ext = "." + (file.name.split(".").pop() || "bin").toLowerCase();
  if (file.size > 20 * 1024 * 1024) {
    throw new Error("附件大小超过 20MB");
  }
  return ext;
}

// 上传文件到服务器源码目录 (POST multipart/form-data)
async function uploadFileToServer(targetDir: string, file: File) {
  validateUpload(file);
  const formData = new FormData();
  formData.append("file", file);
  formData.append("target_dir", targetDir);
  const token = localStorage.getItem("token") || "";
  const res = await fetch("/api/fs/file/upload", {
    method: "POST",
    headers: { "Authorization": `Bearer ${token}` },
    body: formData
  });
  const json = await res.json();
  if (json.code !== 0) {
    throw new Error(json.msg || "上传失败");
  }
  return json.data as { path: string };
}

async function uploadImage(file: File) {
  const currentBook = store.get().currentBook;
  if (!currentBook) {
    notifyError("请先打开一本书中的 Markdown 文件");
    return;
  }
  const ext = validateUpload(file);
  molecule.panel.open({ id: "publish", name: "发布日志", closable: false, renderPane: () => <PublishPane /> });
  const now = Date.now();
  store.set({
    logs: "正在获取云存储上传凭证...",
    publishState: "正在上传附件",
    publishBusy: true,
    publishStartedAt: now,
    publishLastEventAt: now
  });
  const params = await request<{ put_url: string; cdn_img_url: string; content_type: string; acl: string }>(
    `/api/fs/get-s3-upload-params?bookDirName=${encodeURIComponent(currentBook)}&ext=${encodeURIComponent(ext)}`
  );
  store.set({
    logs: `${store.get().logs}\n正在上传附件至云存储...`,
    publishLastEventAt: Date.now()
  });
  // The presigned URL signs "content-type" and "x-amz-acl", so the PUT MUST
  // echo those exact header values back, otherwise S3/COS returns 403.
  const res = await fetch(params.put_url, {
    method: "PUT",
    body: file,
    headers: {
      "Content-Type": params.content_type,
      "x-amz-acl": params.acl
    }
  });
  if (!res.ok) {
    throw new Error("附件上传至云存储失败，请检查网络或文件格式");
  }
  const editor = getCurrentEditorInstance();
  const snippet = buildInsertedSnippet(file, params.cdn_img_url);
  if (editor) {
    const selection = editor.getSelection();
    if (!selection) {
      editor.setValue(`${editor.getValue()}${snippet}`);
    } else {
      editor.executeEdits("image-upload", [{
        range: selection,
        text: snippet,
        forceMoveMarkers: true
      }]);
    }
    editor.focus();
    const nextValue = editor.getValue();
    const currentPath = store.get().currentPath;
    fileContentCache.set(currentPath, nextValue);
    store.set({ currentContent: nextValue });
    // 编辑器路径:插入链接即视为内容变更,同步 edited 状态,确保未保存角标出现
    markTabEdited(currentPath);
  } else {
    store.set({ currentContent: `${store.get().currentContent}${snippet}` });
  }
  store.set({
    logs: `${store.get().logs}\n附件上传完成`,
    publishState: "空闲",
    publishBusy: false,
    publishLastEventAt: Date.now()
  });
  molecule.notification.add([{
    id: `upload-${Date.now()}`,
    value: `附件已上传：${file.name}`,
    status: NotificationStatus.WaitRead
  }]);
}

function openCreateDialog(kind: "file" | "folder", basePath?: string, baseType?: "file" | "folder") {
  const preferred = getPreferredCreationTarget();
  const fallback = resolveCreationBase(
    basePath || preferred.path,
    baseType || preferred.type
  );
  store.set({
    dialog: {
      type: "entry",
      mode: kind,
      title: kind === "file" ? "新建文件" : "新建文件夹",
      label: kind === "file" ? "文件路径" : "文件夹路径",
      confirmText: kind === "file" ? "创建文件" : "创建文件夹",
      value: fallback ? `${fallback}/` : "",
      placeholder: kind === "file" ? "例如 book_demo/chapter-01.md" : "例如 book_demo/assets"
    }
  });
}

function openRenameDialog(path: string) {
  const current = path.split("/").pop() || "";
  store.set({
    dialog: {
      type: "entry",
      mode: "rename",
      title: "重命名",
      label: "新名称",
      confirmText: "保存修改",
      value: current,
      targetPath: path,
      placeholder: current
    }
  });
}

function openDeleteDialog(path: string) {
  store.set({
    dialog: {
      type: "delete",
      title: "确认删除",
      message: `确认删除 ${path} 吗？这个操作不能撤销。`,
      confirmText: "删除",
      path
    }
  });
}

function openLogoutDialog() {
  store.set({
    dialog: {
      type: "delete",
      title: "确认退出登录",
      message: "确认退出当前账号吗？退出后将返回登录页。",
      confirmText: "退出登录",
      path: "__logout__"
    }
  });
}

async function submitEntryDialog(dialog: Extract<DialogState, { type: "entry" }>, rawValue: string) {
  const value = rawValue.trim();
  if (!value) {
    throw new Error("请输入内容");
  }

  const refreshHints = dialog.mode === "rename"
    ? getAncestorPaths(dialog.targetPath || "")
    : getAncestorPaths(value);

  if (dialog.mode === "rename") {
    if (!dialog.targetPath) return;
    const current = dialog.targetPath.split("/").pop() || "";
    if (value === current) return;
    const renamedPath = resolveRenamedPath(dialog.targetPath, value);
    store.set({ dialog: null });
    const result = await request<{ path: string }>("/api/fs/file/rename", {
      method: "PATCH",
      body: JSON.stringify({ path: dialog.targetPath, newName: value })
    });
    const finalPath = result.path || renamedPath;
    syncPathRename(dialog.targetPath, finalPath);
    await refreshAllWithHints([...refreshHints, ...getAncestorPaths(finalPath)], finalPath);
    return;
  }

  store.set({ dialog: null });
  await request("/api/fs/file/new", {
    method: "POST",
    body: JSON.stringify({ type: dialog.mode, path: value })
  });
  store.set({
    treeContextPath: value,
    treeContextType: dialog.mode === "folder" ? "folder" : "file"
  });
  fileContentCache.delete(value);
  await refreshAllWithHints(refreshHints, value);
}

async function submitDeleteDialog(path: string) {
  if (path === "__logout__") {
    await logout();
    return;
  }
  const refreshHints = getAncestorPaths(path);
  store.set({ dialog: null });
  await request("/api/fs/file/remove", {
    method: "DELETE",
    body: JSON.stringify({ path })
  });
  cleanupDeletedPath(path);
  await refreshAllWithHints(refreshHints, refreshHints[refreshHints.length - 1] || "");
}

async function startPublish(mode: "full" | "single", explicitBook?: string) {
  const currentBook = mode === "single"
    ? await ensureBookForPublish(explicitBook)
    : store.get().currentBook;
  if (mode === "single" && !currentBook) return;
  if (currentBook) syncBook(currentBook);
  const now = Date.now();
  store.set({
    logs: "",
    publishURL: "",
    publishState: mode === "full" ? "正在完整发布" : `正在发布 ${currentBook}`,
    publishBusy: true,
    publishStartedAt: now,
    publishLastEventAt: now
  });
  molecule.panel.open({ id: "publish", name: "发布日志", closable: false, renderPane: () => <PublishPane /> });
  const url = mode === "full" ? "/api/publish/full-site" : `/api/publish/single-book?bookDirName=${encodeURIComponent(currentBook)}`;
  const data = await request<{ task_id: string }>(url, { method: "POST" });
  subscribePublish(data.task_id);
}

function subscribePublish(taskId: string) {
  const token = localStorage.getItem("token") || "";
  const stream = new EventSource(`/api/publish/task/stream?taskId=${encodeURIComponent(taskId)}&token=${encodeURIComponent(token)}`);
  stream.addEventListener("log", (event) => {
    const payload = JSON.parse((event as MessageEvent).data) as { line: string };
    const prev = store.get().logs;
    store.set({
      logs: prev ? `${prev}\n${payload.line}` : payload.line,
      publishLastEventAt: Date.now()
    });
  });
  stream.addEventListener("status", (event) => {
    const payload = JSON.parse((event as MessageEvent).data) as TaskInfo;
    store.set({
      publishState: payload.status || "处理中",
      publishURL: payload.result_url || store.get().publishURL,
      publishBusy: !payload.done,
      publishLastEventAt: Date.now()
    });
    if (payload.error_msg) notifyError(payload.error_msg);
    if (payload.done) {
      stream.close();
      void refreshAll();
    }
  });
  stream.onerror = () => {
    stream.close();
    store.set({
      publishState: "发布流已断开",
      publishBusy: false,
      publishLastEventAt: Date.now()
    });
  };
}

async function logout() {
  await request("/api/logout", { method: "POST" });
  localStorage.removeItem("token");
  location.href = "/login.html";
}

function syncBook(path?: string) {
  const book = detectBookFromPath(path);
  if (!book) return;
  store.set({ currentBook: book });
  molecule.statusBar.update({
    id: "book",
    name: book,
    render: () => <span>{book}</span>
  }, Float.left);
}

function guessLanguage(path: string) {
  if (isPreviewablePath(path)) return "plaintext";
  if (path.endsWith(".md")) return "markdown";
  if (path.endsWith(".yaml") || path.endsWith(".yml")) return "yaml";
  if (path.endsWith(".toml")) return "ini";
  if (path.endsWith(".json")) return "json";
  return "plaintext";
}

function notifyError(message: string) {
  molecule.notification.add([{ id: `err-${Date.now()}`, value: message, status: NotificationStatus.WaitRead }]);
}

function buildEditorTab(path: string, value: string) {
  const language = guessLanguage(path);
  const isMarkdown = path.endsWith(".md");
  const isImage = isImagePath(path);
  const isPdf = isPdfPath(path);
  return {
    id: path,
    name: getEditorTabName(path),
    data: {
      path,
      value,
      language,
      modified: false
    },
    renderPane: isImage
      ? (_: unknown, tab?: { data?: { path?: string } }) => (
          <ImagePreviewPane path={tab?.data?.path || path} />
        )
      : isPdf
        ? (_: unknown, tab?: { data?: { path?: string } }) => (
            <PdfPreviewPane path={tab?.data?.path || path} />
          )
      : isMarkdown
        ? (_: unknown, tab?: { data?: { path?: string; value?: string } }, group?: { id?: string | number }) => (
          <MarkdownSplitPane
            path={tab?.data?.path || path}
            value={tab?.data?.value || ""}
            groupId={group?.id}
          />
        )
        : undefined
  };
}

function buildInsertedSnippet(file: File, url: string) {
  const currentPath = store.get().currentPath;
  const ext = file.name.split(".").pop()?.toLowerCase() || "";
  const isImage = file.type.startsWith("image/") || ["png", "jpg", "jpeg", "gif", "webp", "svg"].includes(ext);
  const isMarkdownFile = currentPath.endsWith(".md");
  if (!isMarkdownFile) {
    return `\n${url}\n`;
  }
  if (isImage) {
    return `\n![${file.name}](${url})\n`;
  }
  return `\n[附件：${file.name}](${url})\n`;
}

function getEditorTabName(path: string) {
  return path === WORKBENCH_SETTINGS_PATH ? "工作台设置.json" : (path.split("/").pop() || path);
}

function findEditorTabByPath(path: string, preferredGroupId?: string | number) {
  const docState = getDocumentState(path);
  if (docState?.groupId !== undefined) {
    const docTab = molecule.editor.getTabById(path, docState.groupId);
    if (docTab) {
      return {
        groupId: docState.groupId,
        tab: docTab
      };
    }
  }
  if (activeTabPath === path && activeTabGroupId !== undefined) {
    const activeTab = molecule.editor.getTabById(path, activeTabGroupId);
    if (activeTab) {
      return {
        groupId: activeTabGroupId,
        tab: activeTab
      };
    }
  }
  const current = molecule.editor.getState().current;
  if (current?.tab?.data?.path === path && current.id !== undefined) {
    return {
      groupId: current.id,
      tab: current.tab
    };
  }
  const triedGroupIds = Array.from(new Set([
    preferredGroupId,
    molecule.editor.getGroupIdByTab(path)
  ].filter((groupId): groupId is string | number => groupId !== null && groupId !== undefined)));
  for (const groupId of triedGroupIds) {
    const tab = molecule.editor.getTabById(path, groupId);
    if (!tab) continue;
    return {
      groupId,
      tab
    };
  }
  return null;
}

function syncEditorTabState(
  path: string,
  next: {
    value?: string;
    modified: boolean;
  },
  preferredGroupId?: string | number
) {
  const target = findEditorTabByPath(path, preferredGroupId);
  if (!target) return;
  const currentData = target.tab.data || {};
  const docState = updateDocumentState(path, {
    content: next.value ?? currentData.value ?? fileContentCache.get(path) ?? store.get().currentContent,
    modified: next.modified,
    groupId: preferredGroupId ?? target.groupId
  });
  const nextValue = docState?.content ?? currentData.value ?? fileContentCache.get(path) ?? store.get().currentContent;
  if (currentData.modified === next.modified && currentData.value === nextValue && target.tab.status === (next.modified ? "edited" : undefined)) {
    return;
  }
  molecule.editor.updateTab({
    ...target.tab,
    name: getEditorTabName(path),
    status: next.modified ? "edited" : undefined,
    data: {
      ...currentData,
      path,
      value: nextValue,
      modified: next.modified
    }
  }, target.groupId);
}

async function moveTreeEntry(source: IFolderTreeNodeProps, target: IFolderTreeNodeProps) {
  const sourcePath = source.location;
  const targetPath = target.location || "";
  if (!sourcePath) return;
  const targetIsFolder = target.fileType === "Folder" || target.fileType === "RootFolder";
  const targetDir = target.fileType === "RootFolder" ? "" : (targetIsFolder ? targetPath : dirname(targetPath));
  const nextPath = joinPath(targetDir, basename(sourcePath));
  if (!nextPath || nextPath === sourcePath) return;
  if (source.fileType !== "File" && isChildPath(sourcePath, nextPath)) {
    notifyError("不能把文件夹移动到它自己的子目录中");
    return;
  }
  const result = await request<{ path: string }>("/api/fs/file/rename", {
    method: "PATCH",
    body: JSON.stringify({ path: sourcePath, newName: nextPath })
  });
  const finalPath = result.path || nextPath;
  syncPathRename(sourcePath, finalPath);
  store.set({
    treeContextPath: finalPath,
    treeContextType: source.fileType === "File" ? "file" : "folder"
  });
  await refreshAllWithHints([...getAncestorPaths(sourcePath), ...getAncestorPaths(finalPath)], finalPath);
}

async function duplicateTreeEntry(path: string, type: "file" | "folder") {
  const result = await request<{ path: string }>("/api/fs/file/copy", {
    method: "POST",
    body: JSON.stringify({ path })
  });
  const finalPath = result.path || path;
  store.set({
    treeContextPath: finalPath,
    treeContextType: type
  });
  fileContentCache.delete(finalPath);
  await refreshAllWithHints([...getAncestorPaths(path), ...getAncestorPaths(finalPath)], finalPath);
}

function updateTabValue(path: string, value: string, modified: boolean, groupId?: string | number) {
  syncEditorTabState(path, { value, modified }, groupId);
}

function markTabEdited(path: string, groupId?: string | number) {
  const latestValue = getDocumentState(path)?.content ?? fileContentCache.get(path) ?? store.get().currentContent;
  syncEditorTabState(path, { value: latestValue, modified: true }, groupId);
}

function setActiveTab(path: string, groupId?: string | number, content?: string) {
  if (!path) return;
  if (groupId !== undefined || typeof content === "string") {
    updateDocumentState(path, { groupId, content });
  }
  activeTabPath = path;
  activeTabGroupId = groupId;
  store.set({
    currentPath: path,
    currentContent: content ?? getDocumentState(path)?.content ?? fileContentCache.get(path) ?? store.get().currentContent
  });
}

function bindEditorToPath(path: string, editor: MonacoEditorNS.IStandaloneCodeEditor) {
  if (!path) return;
  Array.from(editorInstanceByPath.entries()).forEach(([mappedPath, mappedEditor]) => {
    if (mappedEditor === editor && mappedPath !== path) {
      editorInstanceByPath.delete(mappedPath);
    }
  });
  editorInstanceByPath.set(path, editor);
}

function setActiveEditor(path: string, editor: MonacoEditorNS.IStandaloneCodeEditor) {
  if (!path) return;
  bindEditorToPath(path, editor);
  activeEditorInstance = editor;
  activeEditorPath = path;
}

function getCurrentEditorInstance(expectedPath = store.get().currentPath) {
  if (activeEditorInstance && activeEditorPath === expectedPath) {
    return activeEditorInstance;
  }
  const moleculeEditor = molecule.editor.editorInstance;
  if (moleculeEditor && getActiveTabPath() === expectedPath) {
    return moleculeEditor;
  }
  return null;
}

function getActiveTabPath() {
  return activeTabPath || store.get().currentPath;
}

function getCurrentEditorValue(expectedPath = store.get().currentPath) {
  const boundEditor = editorInstanceByPath.get(expectedPath);
  if (boundEditor) {
    return boundEditor.getValue();
  }
  const docState = getDocumentState(expectedPath);
  if (docState) {
    return docState.content;
  }
  const activeTab = getActiveTabSnapshot();
  if (activeTab.path === expectedPath && typeof activeTab.value === "string") {
    return activeTab.value;
  }
  if (expectedPath === store.get().currentPath) {
    return getCurrentEditorInstance(expectedPath)?.getValue() ?? store.get().currentContent;
  }
  return getCurrentEditorInstance(expectedPath)?.getValue() ?? fileContentCache.get(expectedPath) ?? "";
}

function attachCustomEditorContextMenu(editor: MonacoEditorNS.IStandaloneCodeEditor) {
  const domNode = editor.getDomNode();
  if (!domNode || domNode.dataset.cmsContextMenuBound === "true") return;
  const handler = (event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
    openEditorContextMenu(event.clientX, event.clientY);
  };
  domNode.addEventListener("contextmenu", handler);
  domNode.dataset.cmsContextMenuBound = "true";
}

// 在编辑器上注册全局快捷键。每个编辑器实例都需注册一次,否则切换 tab 后
// 旧的命令绑定会失效(Ctrl+A 偶发失效的根因)。注意不要覆盖 Ctrl+Z / Ctrl+Y
// 等编辑器内置命令,这里只接管保存。
const registeredEditorSet = new WeakSet<MonacoEditorNS.IStandaloneCodeEditor>();
function registerEditorCommands(editor: MonacoEditorNS.IStandaloneCodeEditor) {
  if (registeredEditorSet.has(editor)) return;
  registeredEditorSet.add(editor);
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyS, () => {
    void saveCurrentFile();
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyMod.Shift | KeyCode.KeyP, () => {
    openCommandPaletteNotice();
  });
  // Markdown 自定义 renderPane 不走 Molecule 内置编辑器那条完整快捷键链路，
  // 这里把常用命令显式桥接回 Monaco，避免 Markdown 文件里快捷键失效。
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyA, () => {
    editor.trigger("cms-shortcut", "editor.action.selectAll", undefined);
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyF, () => {
    editor.trigger("cms-shortcut", "actions.find", undefined);
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyH, () => {
    editor.trigger("cms-shortcut", "editor.action.startFindReplaceAction", undefined);
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyZ, () => {
    editor.trigger("cms-shortcut", "undo", undefined);
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyY, () => {
    editor.trigger("cms-shortcut", "redo", undefined);
  });
  editor.addCommand(KeyMod.CtrlCmd | KeyMod.Shift | KeyCode.KeyZ, () => {
    editor.trigger("cms-shortcut", "redo", undefined);
  });
}

// Explorer 面板空白区域右键 -> IDE 菜单。只挂在 Explorer 容器上,页面其它
// 部分(菜单栏/状态栏/预览区等)保持浏览器默认右键行为。
function bindExplorerContextMenu() {
  const handler = (event: MouseEvent) => {
    const target = resolveExplorerContextTargetFromEvent(event);
    explorerContextTarget = target.path ? target : null;
    if (target.path) {
      store.set({
        treeContextPath: target.path,
        treeContextType: target.type || ""
      });
      const book = detectBookFromPath(target.path);
      if (book) syncBook(book);
    }
    event.preventDefault();
    event.stopPropagation();
    window.setTimeout(() => {
      openExplorerContextMenu(event.clientX, event.clientY);
    }, 0);
  };
  document.addEventListener("contextmenu", (event) => {
    const target = event.target as HTMLElement | null;
    if (!target?.closest(".mo-sidebar, .mo-folderTree, .mo-tree, [data-content='sidebar.explore.folders']")) return;
    handler(event);
  }, true);
}

function bindExplorerSelectionSync() {
  document.addEventListener("click", (event) => {
    const target = event.target as HTMLElement | null;
    if (!target?.closest(".mo-sidebar, .mo-folderTree, .mo-tree, [data-content='sidebar.explore.folders']")) return;
    window.setTimeout(() => {
      const preferred = getPreferredCreationTarget();
      explorerContextTarget = preferred.path
        ? { path: preferred.path, type: preferred.type }
        : null;
      const book = detectBookFromPath(preferred.path);
      if (book) syncBook(book);
    }, 0);
  }, true);
}

function openExplorerContextMenu(x: number, y: number) {
  const existing = document.getElementById("cms-explorer-context-menu");
  if (existing) existing.remove();

  const menu = document.createElement("div");
  menu.id = "cms-explorer-context-menu";
  menu.className = "editor-context-menu";
  menu.style.left = `${x}px`;
  menu.style.top = `${y}px`;

  const target = explorerContextTarget || getPreferredCreationTarget();
  const items = buildExplorerContextItems(target.path, target.type);

  items.forEach((item) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "editor-context-menu-item";
    button.textContent = item.label;
    button.onclick = () => {
      removeExplorerContextMenu();
      void item.action();
    };
    menu.appendChild(button);
  });

  document.body.appendChild(menu);
  window.setTimeout(() => {
    document.addEventListener("click", removeExplorerContextMenu, { once: true });
  }, 0);
}

function removeExplorerContextMenu() {
  document.getElementById("cms-explorer-context-menu")?.remove();
}

function buildExplorerContextItems(path: string, type?: "file" | "folder") {
  const isNode = Boolean(path);
  const nodeKind = type || "";
  const items: Array<{ label: string; action: () => void | Promise<void> }> = [];
  if (!isNode || nodeKind === "folder") {
    items.push(
      { label: "新建文件", action: () => openCreateDialog("file", path, nodeKind === "folder" ? "folder" : undefined) },
      { label: "新建文件夹", action: () => openCreateDialog("folder", path, nodeKind === "folder" ? "folder" : undefined) },
      { label: "上传附件", action: () => triggerUploadToServer() }
    );
  }
  if (isNode) {
    items.push({ label: "创建副本", action: () => void duplicateTreeEntry(path, nodeKind === "folder" ? "folder" : "file") });
    items.push({ label: "重命名", action: () => openRenameDialog(path) });
    items.push({ label: "删除", action: () => openDeleteDialog(path) });
  }
  items.push({ label: "刷新", action: () => refreshAll() });
  return items;
}

function resolveExplorerContextTargetFromEvent(event: MouseEvent): { path: string; type?: "file" | "folder" } {
  const element = (event.target as HTMLElement | null)?.closest("[data-key]") as HTMLElement | null;
  const rawKey = element?.dataset.key || "";
  if (!rawKey || rawKey === "source_root") {
    return { path: "", type: undefined };
  }
  const node = findTreeNodeByPath(rawKey, currentTree);
  if (!node) {
    return { path: rawKey, type: undefined };
  }
  return {
    path: node.path,
    type: node.type === "folder" ? "folder" : "file"
  };
}

function findTreeNodeByPath(path: string, nodes: TreeNode[]): TreeNode | null {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.children?.length) {
      const child = findTreeNodeByPath(path, node.children);
      if (child) return child;
    }
  }
  return null;
}

function isImagePath(path: string) {
  return /\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(path);
}

function isPdfPath(path: string) {
  return /\.pdf$/i.test(path);
}

function isPreviewablePath(path: string) {
  return isImagePath(path) || isPdfPath(path);
}

function renderMarkdown(content: string) {
  return marked.parse(content, { async: false }) as string;
}

function resolveCreationBase(path: string, pathType?: "file" | "folder") {
  if (!path) return "";
  const cleaned = path.replace(/\/+$/, "");
  if (!cleaned) return "";
  if (pathType === "folder") return cleaned;
  if (pathType === "file") {
    return cleaned.includes("/") ? cleaned.slice(0, cleaned.lastIndexOf("/")) : "";
  }
  if (cleaned.includes(".") && !cleaned.endsWith(".md")) {
    return cleaned.includes("/") ? cleaned.slice(0, cleaned.lastIndexOf("/")) : "";
  }
  if (cleaned.endsWith(".md") || cleaned.endsWith(".json") || cleaned.endsWith(".yaml") || cleaned.endsWith(".yml") || cleaned.endsWith(".toml")) {
    return cleaned.includes("/") ? cleaned.slice(0, cleaned.lastIndexOf("/")) : "";
  }
  return cleaned;
}

async function runWorkspaceSearch(keyword: string) {
  const query = keyword.trim();
  if (!query) {
    molecule.search.setResult([]);
    molecule.search.setValidateInfo("");
    return;
  }

  const files = flattenTreeFiles(currentTree).filter((path) => isSearchableFile(path));
  molecule.search.setValidateInfo({ type: "info", text: "正在搜索..." });
  const groups = await Promise.all(files.map(async (path) => {
    const content = await getFileContentCached(path);
    return buildSearchItems(path, content, query);
  }));
  const results = groups.flat().slice(0, 300);
  molecule.search.setResult(results);
  molecule.search.setValidateInfo({ type: "info", text: `找到 ${results.length} 条结果` });
}

function flattenTreeFiles(tree: TreeNode[]) {
  const files: string[] = [];
  const walk = (nodes: TreeNode[]) => {
    nodes.forEach((node) => {
      if (node.type === "file") {
        files.push(node.path);
        return;
      }
      if (node.children?.length) walk(node.children);
    });
  };
  walk(tree);
  return files;
}

function isSearchableFile(path: string) {
  return /\.(md|txt|json|ya?ml|toml|html?|css|scss|js|ts|tsx|jsx)$/i.test(path);
}

function isEditablePath(path: string) {
  return /\.(md|txt|json|ya?ml|toml|html?|css|scss|js|ts|tsx|jsx|php)$/i.test(path) || path === WORKBENCH_SETTINGS_PATH;
}

function buildRawFileURL(path: string) {
  const token = localStorage.getItem("token") || "";
  return `/api/fs/file/raw?path=${encodeURIComponent(path)}&token=${encodeURIComponent(token)}`;
}

async function getFileContentCached(path: string) {
  if (path === WORKBENCH_SETTINGS_PATH) {
    return JSON.stringify(getCurrentWorkbenchSettings(), null, 2);
  }
  if (fileContentCache.has(path)) {
    return fileContentCache.get(path) || "";
  }
  try {
    const data = await request<{ content: string }>(`/api/fs/file/content?path=${encodeURIComponent(path)}`);
    fileContentCache.set(path, data.content);
    return data.content;
  } catch {
    return "";
  }
}

function buildSearchItems(path: string, content: string, keyword: string): IFolderTreeNodeProps[] {
  if (!content) return [];
  const lines = content.split(/\r?\n/);
  const items: IFolderTreeNodeProps[] = [];
  lines.forEach((line, index) => {
    const matchIndex = getSearchIndexForText(line, keyword);
    if (matchIndex < 0) return;
    items.push({
      id: `${path}:${index + 1}:${matchIndex + 1}`,
      name: `${path} · 第 ${index + 1} 行 · ${line.trim() || "(空行)"}`,
      isLeaf: true,
      data: {
        path,
        lineNumber: index + 1,
        column: matchIndex + 1,
        matchLength: keyword.length
      } satisfies SearchLeafData
    });
  });
  return items;
}

function getSearchIndexForText(text: string, query: string) {
  const state = molecule.search.getState();
  try {
    if (state.isRegex) {
      const source = state.isWholeWords ? `\\b${query}\\b` : query;
      const flags = state.isCaseSensitive ? "" : "i";
      return text.search(new RegExp(source, flags));
    }
    if (state.isWholeWords) {
      const source = `\\b${escapeRegExp(query)}\\b`;
      const flags = state.isCaseSensitive ? "" : "i";
      return text.search(new RegExp(source, flags));
    }
    if (state.isCaseSensitive) {
      return text.indexOf(query);
    }
    return text.toLowerCase().indexOf(query.toLowerCase());
  } catch {
    return -1;
  }
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function focusEditorLocation(path: string, location: OpenFileOptions, retry = 0) {
  window.setTimeout(() => {
    if (store.get().currentPath !== path) {
      if (retry < 6) focusEditorLocation(path, location, retry + 1);
      return;
    }
    const editor = getCurrentEditorInstance();
    if (!editor) {
      if (retry < 6) focusEditorLocation(path, location, retry + 1);
      return;
    }
    const lineNumber = location.lineNumber || 1;
    const column = location.column || 1;
    const endColumn = column + Math.max(location.matchLength || 1, 1);
    editor.focus();
    editor.revealLineInCenter(lineNumber);
    editor.setPosition({ lineNumber, column });
    editor.setSelection({
      startLineNumber: lineNumber,
      startColumn: column,
      endLineNumber: lineNumber,
      endColumn
    });
  }, retry === 0 ? 80 : 120);
}

function openEditorContextMenu(x: number, y: number) {
  const existing = document.getElementById("cms-editor-context-menu");
  if (existing) existing.remove();

  const menu = document.createElement("div");
  menu.id = "cms-editor-context-menu";
  menu.className = "editor-context-menu";
  menu.style.left = `${x}px`;
  menu.style.top = `${y}px`;

  const items = [
    { label: "上传附件", action: () => triggerUploadImage() },
    { label: "保存", action: () => saveCurrentFile() },
    { label: "复制", action: () => copyEditorSelection() },
    { label: "粘贴", action: () => pasteIntoEditor() },
    { label: "全选", action: () => selectAllEditorContent() }
  ];

  items.forEach((item) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "editor-context-menu-item";
    button.textContent = item.label;
    button.onclick = async () => {
      removeEditorContextMenu();
      await item.action();
    };
    menu.appendChild(button);
  });

  document.body.appendChild(menu);
  window.setTimeout(() => {
    document.addEventListener("click", removeEditorContextMenu, { once: true });
  }, 0);
}

function removeEditorContextMenu() {
  document.getElementById("cms-editor-context-menu")?.remove();
}

async function copyEditorSelection() {
  const editor = getCurrentEditorInstance();
  if (!editor) return;
  const selection = editor.getSelection();
  const text = selection ? editor.getModel()?.getValueInRange(selection) || "" : editor.getValue();
  if (!text) return;
  await navigator.clipboard.writeText(text);
}

async function pasteIntoEditor() {
  const editor = getCurrentEditorInstance();
  if (!editor) return;
  const text = await navigator.clipboard.readText();
  const selection = editor.getSelection();
  if (!selection) {
    editor.setValue(`${editor.getValue()}${text}`);
    return;
  }
  editor.executeEdits("paste-from-menu", [{ range: selection, text, forceMoveMarkers: true }]);
}

function selectAllEditorContent() {
  const editor = getCurrentEditorInstance();
  const model = editor?.getModel();
  if (!editor || !model) return;
  const lastLine = model.getLineCount();
  const lastColumn = model.getLineMaxColumn(lastLine);
  editor.setSelection({
    startLineNumber: 1,
    startColumn: 1,
    endLineNumber: lastLine,
    endColumn: lastColumn
  });
  editor.focus();
}

async function handleActionDialog(actionId: string) {
  store.set({ dialog: null });
  if (actionId.startsWith("publish-book:")) {
    await startPublish("single", actionId.slice("publish-book:".length));
    return;
  }
  if (actionId.startsWith("theme:")) {
    molecule.colorTheme.setTheme(actionId.slice("theme:".length));
    return;
  }
  if (actionId === ACTION_QUICK_COMMAND) {
    openCommandPaletteNotice();
    return;
  }
  if (actionId === ACTION_QUICK_ACCESS_SETTINGS) {
    openWorkbenchSettingsEditor();
    return;
  }
  if (actionId === ACTION_THEME_DIALOG) {
    openThemeDialog();
    return;
  }
  if (actionId === ACTION_SEARCH_HELP) {
    molecule.notification.add([{
      id: `search-${Date.now()}`,
      value: "搜索支持文本内容检索，点击结果会直接定位到具体行。",
      status: NotificationStatus.WaitRead
    }]);
  }
}

function openWorkbenchActionDialog() {
  store.set({
    dialog: {
      type: "action",
      title: "工作台设置",
      message: "选择你要打开的功能。",
      actions: [
        { id: ACTION_QUICK_COMMAND, label: "命令面板" },
        { id: ACTION_QUICK_ACCESS_SETTINGS, label: "打开设置" },
        { id: ACTION_THEME_DIALOG, label: "主题颜色" },
        { id: ACTION_SEARCH_HELP, label: "搜索说明" }
      ]
    }
  });
}

function openThemeDialog() {
  const themes = molecule.colorTheme.getThemes();
  store.set({
    dialog: {
      type: "action",
      title: "主题颜色",
      message: "选择一个主题，结果会自动保存到本地。",
      actions: themes.map((theme) => ({
        id: `theme:${theme.id}`,
        label: theme.label || theme.name || theme.id
      }))
    }
  });
}

function setupUiLocalization() {
  const apply = () => {
    localizeSearchUi();
    localizeSettingsTab();
  };
  apply();
  const observer = new MutationObserver(() => apply());
  observer.observe(document.body, { childList: true, subtree: true });
}

function localizeSearchUi() {
  document.querySelectorAll<HTMLInputElement>(".mo-search input").forEach((input, index) => {
    const placeholder = index === 0 ? "搜索内容" : "替换内容";
    if (input.placeholder !== placeholder) {
      input.placeholder = placeholder;
    }
  });
}

function localizeSettingsTab() {
  const groups = (molecule.editor.getState().groups || []) as Array<{ id: string | number; data?: Array<{ id: string | number; name?: string }> }>;
  groups.forEach((group) => {
    group.data?.forEach((tab) => {
      if (tab.id !== WORKBENCH_SETTINGS_PATH || tab.name === "工作台设置.json") return;
      const currentTab = molecule.editor.getTabById(tab.id, group.id);
      if (!currentTab) return;
      molecule.editor.updateTab({ ...currentTab, name: "工作台设置.json" }, group.id);
    });
  });
}

function setupOverflowImprovements() {
  const syncActiveTabIntoView = () => {
    window.requestAnimationFrame(() => {
      document.querySelector<HTMLElement>(".mo-editor .mo-tab__item--active")?.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "nearest"
      });
    });
  };

  const bindTabWheelScroll = () => {
    document.querySelectorAll<HTMLElement>(".mo-editor__group-tabs .mo-scrollBar__wrapper").forEach((wrapper) => {
      if (wrapper.dataset.cmsWheelBound === "true") return;
      wrapper.dataset.cmsWheelBound = "true";
      wrapper.addEventListener("wheel", (event) => {
        if (Math.abs(event.deltaY) <= Math.abs(event.deltaX) || !wrapper.scrollWidth || wrapper.scrollWidth <= wrapper.clientWidth) {
          return;
        }
        event.preventDefault();
        wrapper.scrollLeft += event.deltaY;
      }, { passive: false });
    });
  };

  const apply = () => {
    bindTabWheelScroll();
    syncActiveTabIntoView();
  };

  apply();
  const observer = new MutationObserver(() => apply());
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["class"] });
}

function openWorkbenchSettingsEditor() {
  const content = JSON.stringify(getCurrentWorkbenchSettings(), null, 2);
  store.set({
    currentPath: WORKBENCH_SETTINGS_PATH,
    currentContent: content
  });
  const groupId = molecule.editor.getGroupIdByTab(WORKBENCH_SETTINGS_PATH);
  const existing = groupId !== null ? molecule.editor.getTabById(WORKBENCH_SETTINGS_PATH, groupId) : undefined;
  const tab = buildEditorTab(WORKBENCH_SETTINGS_PATH, content);
  if (existing && groupId !== null) {
    molecule.editor.updateTab({
      ...existing,
      ...tab,
      name: getEditorTabName(WORKBENCH_SETTINGS_PATH),
      status: undefined,
      data: {
        ...(existing.data || {}),
        ...(tab.data || {}),
        modified: false
      }
    }, groupId);
    molecule.editor.setActive(groupId, WORKBENCH_SETTINGS_PATH);
    return;
  }
  molecule.editor.open(tab);
}

function getCurrentWorkbenchSettings() {
  return {
    colorTheme: molecule.colorTheme.getColorTheme().id,
    locale: "zh-CN-cms",
    editor: {
      ...molecule.editor.getState().editorOptions,
      contextmenu: false
    }
  };
}

function applyWorkbenchSettings(raw: string) {
  let parsed: {
    colorTheme?: string;
    locale?: string;
    editor?: MonacoEditorNS.IEditorOptions;
  };
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new Error("设置 JSON 格式不正确");
  }
  if (parsed.colorTheme) {
    const theme = molecule.colorTheme.getThemeById(parsed.colorTheme);
    if (!theme) {
      throw new Error(`主题不存在: ${parsed.colorTheme}`);
    }
    molecule.colorTheme.setTheme(parsed.colorTheme);
  }
  if (parsed.editor) {
    molecule.editor.updateEditorOptions({
      ...parsed.editor,
      contextmenu: false
    });
  }
  const normalized = JSON.stringify(getCurrentWorkbenchSettings(), null, 2);
  localStorage.setItem("cms.workbench.theme", molecule.colorTheme.getColorTheme().id);
  store.set({
    currentPath: WORKBENCH_SETTINGS_PATH,
    currentContent: normalized
  });
}

function markCurrentTabSaved(content: string) {
  const currentPath = getActiveTabPath();
  if (!currentPath) return;
  updateDocumentState(currentPath, { content, modified: false, groupId: activeTabGroupId });
  store.set({ currentContent: content });
  fileContentCache.set(currentPath, content);
  syncEditorTabState(currentPath, { value: content, modified: false });
}

function installUnsavedCloseGuards() {
  if (closeGuardsInstalled) return;
  closeGuardsInstalled = true;
  const originalCloseTab = molecule.editor.closeTab.bind(molecule.editor);
  const originalCloseAll = molecule.editor.closeAll.bind(molecule.editor);
  const originalCloseOther = molecule.editor.closeOther.bind(molecule.editor);
  const originalCloseToLeft = molecule.editor.closeToLeft.bind(molecule.editor);
  const originalCloseToRight = molecule.editor.closeToRight.bind(molecule.editor);

  molecule.editor.closeTab = ((tabId: string | number, groupId: string | number) => {
    const tab = molecule.editor.getTabById(tabId, groupId);
    if (!canCloseTabs(tab ? [tab] : [])) return;
    originalCloseTab(tabId, groupId);
  }) as typeof molecule.editor.closeTab;

  molecule.editor.closeAll = ((groupId: string | number) => {
    const tabs = getGroupTabs(groupId);
    if (!canCloseTabs(tabs)) return;
    originalCloseAll(groupId);
  }) as typeof molecule.editor.closeAll;

  molecule.editor.closeOther = ((tab: { id: string | number }, groupId: string | number) => {
    const tabs = getGroupTabs(groupId).filter((item) => item.id !== tab.id);
    if (!canCloseTabs(tabs)) return;
    originalCloseOther(tab as never, groupId);
  }) as typeof molecule.editor.closeOther;

  molecule.editor.closeToLeft = ((tab: { id: string | number }, groupId: string | number) => {
    const tabs = getTabsToLeft(tab.id, groupId);
    if (!canCloseTabs(tabs)) return;
    originalCloseToLeft(tab as never, groupId);
  }) as typeof molecule.editor.closeToLeft;

  molecule.editor.closeToRight = ((tab: { id: string | number }, groupId: string | number) => {
    const tabs = getTabsToRight(tab.id, groupId);
    if (!canCloseTabs(tabs)) return;
    originalCloseToRight(tab as never, groupId);
  }) as typeof molecule.editor.closeToRight;
}

function withCloseGuardBypass(action: () => void) {
  closeGuardBypassDepth += 1;
  try {
    action();
  } finally {
    closeGuardBypassDepth -= 1;
  }
}

function canCloseTabs(tabs: Array<{ id?: string | number; name?: string; data?: { path?: string; modified?: boolean } | undefined; status?: string }>) {
  if (closeGuardBypassDepth > 0) return true;
  const modifiedTabs = tabs.filter(isTabModified);
  if (!modifiedTabs.length) return true;
  const names = modifiedTabs.map((tab) => tab.name || basename(tab.data?.path || String(tab.id || "未命名文件")));
  const message = names.length === 1
    ? `文件“${names[0]}”尚未保存，关闭后内容将丢失。是否仍然关闭？`
    : `以下 ${names.length} 个文件尚未保存，关闭后内容将丢失：\n\n${names.join("\n")}\n\n是否仍然关闭？`;
  return window.confirm(message);
}

function isTabModified(tab: { data?: { modified?: boolean; path?: string } | undefined; status?: string; name?: string }) {
  const path = tab.data?.path || "";
  if (path === WORKBENCH_SETTINGS_PATH) return Boolean(tab.data?.modified);
  return Boolean(tab.data?.modified || tab.status === "edited");
}

function getGroupTabs(groupId: string | number) {
  const group = (molecule.editor.getState().groups || []).find((item: { id: string | number }) => item.id === groupId) as
    | { data?: Array<{ id: string | number; name?: string; data?: { path?: string; modified?: boolean }; status?: string }> }
    | undefined;
  return group?.data ? [...group.data] : [];
}

function getTabsToLeft(tabId: string | number, groupId: string | number) {
  const tabs = getGroupTabs(groupId);
  const index = tabs.findIndex((tab) => tab.id === tabId);
  return index > 0 ? tabs.slice(0, index) : [];
}

function getTabsToRight(tabId: string | number, groupId: string | number) {
  const tabs = getGroupTabs(groupId);
  const index = tabs.findIndex((tab) => tab.id === tabId);
  return index >= 0 ? tabs.slice(index + 1) : [];
}

async function refreshAllWithHints(extraPaths: string[] = [], activePath = "") {
  const expandedKeys = Array.from(new Set([
    ...currentExpandKeys,
    ...extraPaths.flatMap(getAncestorPaths)
  ].filter(Boolean)));
  const [siteTree, books] = await Promise.all([
    request<TreeNode[]>("/api/fs/site/root-tree"),
    request<BookItem[]>("/api/fs/book/list")
  ]);
  currentTree = siteTree;
  renderTree(siteTree, expandedKeys, [...extraPaths, activePath]);
  if (!store.get().currentBook && books[0]) {
    syncBook(books[0].book_dir_name);
  }
}

function detectBookFromPath(path?: string) {
  const cleaned = String(path || "").replace(/^\/+/, "");
  const match = cleaned.match(/^(book_[^/]+)/);
  return match?.[1] || "";
}

function getAncestorPaths(path: string) {
  const cleaned = path.replace(/^\/+|\/+$/g, "");
  if (!cleaned) return [];
  const parts = cleaned.split("/");
  const ancestors: string[] = [];
  for (let index = 0; index < parts.length - 1; index += 1) {
    ancestors.push(parts.slice(0, index + 1).join("/"));
  }
  return ancestors;
}

function basename(path: string) {
  const cleaned = path.replace(/\/+$/, "");
  return cleaned.split("/").pop() || cleaned;
}

function dirname(path: string) {
  const cleaned = path.replace(/\/+$/, "");
  const index = cleaned.lastIndexOf("/");
  if (index < 0) return "";
  return cleaned.slice(0, index);
}

function joinPath(base: string, leaf: string) {
  return [base.replace(/\/+$/g, ""), leaf.replace(/^\/+/g, "")].filter(Boolean).join("/");
}

function isChildPath(parent: string, child: string) {
  if (!parent || !child) return false;
  return child.startsWith(`${parent.replace(/\/+$/g, "")}/`);
}

function replacePathPrefix(value: string | undefined, sourcePath: string, nextPath: string) {
  if (!value) return "";
  if (value === sourcePath) return nextPath;
  if (isChildPath(sourcePath, value)) {
    return value.replace(sourcePath, nextPath);
  }
  return value;
}

function getPreferredCreationTarget() {
  const liveNode = (molecule.folderTree.getState().folderTree?.current || null) as IFolderTreeNodeProps | null;
  const liveType = liveNode?.fileType === "Folder" || liveNode?.fileType === "RootFolder"
    ? "folder"
    : liveNode?.fileType === "File"
      ? "file"
      : undefined;
  return {
    path: liveNode?.location || store.get().treeContextPath || store.get().currentPath || store.get().currentBook || "",
    type: liveType || store.get().treeContextType || undefined
  };
}

async function ensureBookForPublish(explicitBook?: string) {
  const fromExplicit = detectBookFromPath(explicitBook || "");
  if (fromExplicit) return fromExplicit;
  const preferred = getPreferredCreationTarget();
  const fromPreferred = detectBookFromPath(preferred.path);
  if (fromPreferred) return fromPreferred;
  const fromCurrent = detectBookFromPath(store.get().currentPath || store.get().currentBook);
  if (fromCurrent) return fromCurrent;
  const books = await request<BookItem[]>("/api/fs/book/list");
  if (books.length === 1) {
    syncBook(books[0].book_dir_name);
    return books[0].book_dir_name;
  }
  store.set({
    dialog: {
      type: "action",
      title: "选择要发布的书籍",
      message: "当前无法自动判断书籍，请先选择一个书籍目录，或直接在这里选择后继续发布。",
      actions: books.map((book) => ({
        id: `publish-book:${book.book_dir_name}`,
        label: book.book_dir_name
      }))
    }
  });
  return "";
}

function resolveRenamedPath(currentPath: string, nextValue: string) {
  if (nextValue.includes("/")) {
    return nextValue.replace(/^\/+|\/+$/g, "");
  }
  return joinPath(dirname(currentPath), nextValue);
}

function syncPathRename(sourcePath: string, nextPath: string) {
  renameFileCachePath(sourcePath, nextPath);
  renameOpenTabsPath(sourcePath, nextPath);
  const state = store.get();
  const nextStorePatch: Partial<AppState> = {};
  const renamedCurrentPath = replacePathPrefix(state.currentPath, sourcePath, nextPath);
  const renamedTreeContextPath = replacePathPrefix(state.treeContextPath, sourcePath, nextPath);
  if (renamedCurrentPath && renamedCurrentPath !== state.currentPath) nextStorePatch.currentPath = renamedCurrentPath;
  if (renamedTreeContextPath && renamedTreeContextPath !== state.treeContextPath) nextStorePatch.treeContextPath = renamedTreeContextPath;
  if (Object.keys(nextStorePatch).length) {
    store.set(nextStorePatch);
  }
  activeEditorPath = replacePathPrefix(activeEditorPath, sourcePath, nextPath);
}

function cleanupDeletedPath(path: string) {
  removeFileCachePath(path);
  Array.from(documentStateByPath.keys()).forEach((key) => {
    if (key === path || isChildPath(path, key)) {
      documentStateByPath.delete(key);
    }
  });
  closeTabsByPath(path);
  const state = store.get();
  const parentPath = dirname(path);
  if (state.currentPath === path || isChildPath(path, state.currentPath)) {
    store.set({
      currentPath: "",
      currentContent: "",
      treeContextPath: parentPath,
      treeContextType: parentPath ? "folder" : ""
    });
    return;
  }
  if (state.treeContextPath === path || isChildPath(path, state.treeContextPath)) {
    store.set({
      treeContextPath: parentPath,
      treeContextType: parentPath ? "folder" : ""
    });
  }
}

function renameFileCachePath(sourcePath: string, nextPath: string) {
  Array.from(fileContentCache.entries()).forEach(([key, value]) => {
    if (key !== sourcePath && !isChildPath(sourcePath, key)) return;
    fileContentCache.delete(key);
    fileContentCache.set(key.replace(sourcePath, nextPath), value);
  });
  Array.from(documentStateByPath.entries()).forEach(([key, value]) => {
    if (key !== sourcePath && !isChildPath(sourcePath, key)) return;
    documentStateByPath.delete(key);
    documentStateByPath.set(key.replace(sourcePath, nextPath), value);
  });
}

function removeFileCachePath(path: string) {
  Array.from(fileContentCache.keys()).forEach((key) => {
    if (key === path || isChildPath(path, key)) {
      fileContentCache.delete(key);
    }
  });
}

function renameOpenTabsPath(sourcePath: string, nextPath: string) {
  const groups = (molecule.editor.getState().groups || []) as Array<{
    id: string | number;
    data?: Array<{
      id: string | number;
      name?: string;
      data?: { path?: string; value?: string; modified?: boolean; language?: string };
    }>;
  }>;
  groups.forEach((group) => {
    group.data?.forEach((tab) => {
      const tabPath = tab.data?.path;
      if (!tabPath || (tabPath !== sourcePath && !isChildPath(sourcePath, tabPath))) return;
      const renamedPath = tabPath.replace(sourcePath, nextPath);
      const currentTab = molecule.editor.getTabById(tab.id, group.id);
      if (!currentTab) return;
      withCloseGuardBypass(() => {
        molecule.editor.closeTab(tab.id, group.id);
        molecule.editor.open({
          ...currentTab,
          id: renamedPath,
          name: renamedPath === WORKBENCH_SETTINGS_PATH ? "工作台设置.json" : basename(renamedPath),
          data: {
            ...(currentTab.data || {}),
            path: renamedPath
          }
        }, group.id);
      });
    });
  });
}

function closeTabsByPath(path: string) {
  const groups = (molecule.editor.getState().groups || []) as Array<{
    id: string | number;
    data?: Array<{ id: string | number; data?: { path?: string } }>;
  }>;
  groups.forEach((group) => {
    group.data?.forEach((tab) => {
      const tabPath = tab.data?.path;
      if (!tabPath || (tabPath !== path && !isChildPath(path, tabPath))) return;
      withCloseGuardBypass(() => {
        molecule.editor.closeTab(tab.id, group.id);
      });
    });
  });
}

function openCommandPaletteNotice() {
  try {
    container.resolve(MonacoService).commandService.executeCommand(CommandQuickAccessViewAction.ID);
  } catch {
    molecule.notification.add([{
      id: `cmd-${Date.now()}`,
      value: "可通过左下角设置图标打开“命令面板 / 设置 / 主题颜色”。",
      status: NotificationStatus.WaitRead
    }]);
  }
}

createRoot(document.getElementById("root")!).render(<App />);
