import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { marked } from "marked";
import { request } from "./request/client";

type TreeNode = { name: string; path: string; type: "file" | "folder"; children?: TreeNode[] };
type BookItem = { book_dir_name: string; weight: number; enable_home_show: boolean };
type RecordItem = { record_id: string; publishing_time: string; publishing_type: string; status: string; public_url: string };
type TaskInfo = { id: string; status: string; result_url: string; error_msg: string; done: boolean };

function App() {
  const [tree, setTree] = useState<TreeNode[]>([]);
  const [content, setContent] = useState("");
  const [currentPath, setCurrentPath] = useState("");
  const [books, setBooks] = useState<BookItem[]>([]);
  const [bookDir, setBookDir] = useState("");
  const [logs, setLogs] = useState("");
  const [records, setRecords] = useState<RecordItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [publishState, setPublishState] = useState("空闲");
  const [selectedPath, setSelectedPath] = useState("");
  const [publishURL, setPublishURL] = useState("");
  const editorRef = useRef<HTMLTextAreaElement>(null);

  async function loadTreeAndBooks() {
    const [siteTree, bookList] = await Promise.all([
      request<TreeNode[]>("/api/fs/site/root-tree"),
      request<BookItem[]>("/api/fs/book/list")
    ]);
    setTree(siteTree);
    setBooks(bookList);
    setBookDir((prev) => prev || bookList[0]?.book_dir_name || "");
  }

  async function loadInit() {
    const [publishRecords] = await Promise.all([
      request<RecordItem[]>("/api/publish/record/list"),
      loadTreeAndBooks()
    ]);
    setRecords(publishRecords);
  }

  useEffect(() => { loadInit().catch(console.error); }, []);

  async function openFile(path: string) {
    const data = await request<{ content: string }>(`/api/fs/file/content?path=${encodeURIComponent(path)}`);
    setCurrentPath(path);
    setSelectedPath(path);
    setContent(data.content);
    const match = path.match(/^(book_[^/]+)/);
    if (match) setBookDir(match[1]);
  }

  async function saveFile() {
    if (!currentPath) return;
    setLoading(true);
    try {
      await request("/api/fs/file/save", { method: "PUT", body: JSON.stringify({ path: currentPath, content }) });
    } finally {
      setLoading(false);
    }
  }

  function validateImage(file: File) {
    const ext = "." + (file.name.split(".").pop() || "png").toLowerCase();
    const allow = [".png", ".jpg", ".jpeg", ".gif", ".webp"];
    if (!allow.includes(ext)) throw new Error("图片格式不支持");
    if (file.size > 20 * 1024 * 1024) throw new Error("图片大小超过 20MB");
    return ext;
  }

  async function uploadImage(file: File) {
    const ext = validateImage(file);
    setLoading(true);
    try {
      const data = await request<{ put_url: string; cdn_img_url: string }>(`/api/fs/get-s3-upload-params?bookDirName=${encodeURIComponent(bookDir)}&ext=${encodeURIComponent(ext)}`);
      const res = await fetch(data.put_url, { method: "PUT", body: file, headers: { "Content-Type": file.type || "application/octet-stream" } });
      if (!res.ok) throw new Error("图片上传至云存储失败，请检查网络或图片格式");
      insertAtCursor(`\n![图片](${data.cdn_img_url})\n`);
    } finally {
      setLoading(false);
    }
  }

  function insertAtCursor(text: string) {
    const el = editorRef.current;
    if (!el) {
      setContent((prev) => prev + text);
      return;
    }
    const start = el.selectionStart;
    const end = el.selectionEnd;
    const next = content.slice(0, start) + text + content.slice(end);
    setContent(next);
    setTimeout(() => {
      el.focus();
      el.selectionStart = el.selectionEnd = start + text.length;
    }, 0);
  }

  async function onPaste(e: React.ClipboardEvent<HTMLTextAreaElement>) {
    const file = Array.from(e.clipboardData.items).find((item) => item.type.startsWith("image/"))?.getAsFile();
    if (!file) return;
    e.preventDefault();
    try {
      await uploadImage(file);
    } catch (error) {
      alert((error as Error).message);
    }
  }

  async function triggerUpload() {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".png,.jpg,.jpeg,.gif,.webp";
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      try {
        await uploadImage(file);
      } catch (error) {
        alert((error as Error).message);
      }
    };
    input.click();
  }

  async function publish(mode: "full" | "single") {
    setLoading(true);
    setLogs("");
    setPublishURL("");
    setPublishState(mode === "full" ? "准备完整发布" : `准备发布 ${bookDir}`);
    try {
      const url = mode === "full" ? "/api/publish/full-site" : `/api/publish/single-book?bookDirName=${encodeURIComponent(bookDir)}`;
      const data = await request<{ task_id: string }>(url, { method: "POST" });
      subscribePublish(data.task_id);
    } finally {
      setLoading(false);
    }
  }

  function subscribePublish(taskID: string) {
    const token = localStorage.getItem("token") || "";
    const stream = new EventSource(`/api/publish/task/stream?taskId=${encodeURIComponent(taskID)}&token=${encodeURIComponent(token)}`);
    stream.addEventListener("log", (event) => {
      const payload = JSON.parse((event as MessageEvent).data) as { line: string };
      setLogs((prev) => `${prev}${prev ? "\n" : ""}${payload.line}`);
    });
    stream.addEventListener("status", (event) => {
      const payload = JSON.parse((event as MessageEvent).data) as TaskInfo;
      setPublishState(payload.status);
      if (payload.result_url) setPublishURL(payload.result_url);
      if (payload.done) {
        stream.close();
        loadInit().catch(console.error);
      }
      if (payload.error_msg) {
        alert(payload.error_msg);
      }
    });
    stream.onerror = () => {
      stream.close();
      setPublishState("日志连接已关闭");
    };
  }

  async function showRecord(id: string) {
    const detail = await request<any>(`/api/publish/record/detail?recordId=${encodeURIComponent(id)}`);
    setLogs(detail.full_log);
    setPublishURL(detail.public_url || "");
    setPublishState(detail.status || "历史记录");
  }

  async function createEntry(kind: "file" | "folder") {
    const base = selectedPath && !selectedPath.endsWith(".md") ? selectedPath : bookDir;
    const next = window.prompt(kind === "file" ? "输入新文件路径，如 book_01_前端开发手册/content/new.md" : "输入新目录路径", base ? `${base}/` : "");
    if (!next) return;
    await request("/api/fs/file/new", { method: "POST", body: JSON.stringify({ type: kind, path: next }) });
    await loadTreeAndBooks();
  }

  async function renameEntry() {
    if (!selectedPath) return;
    const current = selectedPath.split("/").pop() || "";
    const next = window.prompt("输入新名称", current);
    if (!next || next === current) return;
    await request("/api/fs/file/rename", { method: "PATCH", body: JSON.stringify({ path: selectedPath, newName: next }) });
    if (currentPath === selectedPath) {
      const parts = selectedPath.split("/");
      parts[parts.length - 1] = next;
      setCurrentPath(parts.join("/"));
    }
    await loadTreeAndBooks();
  }

  async function removeEntry() {
    if (!selectedPath) return;
    if (!window.confirm(`确认删除 ${selectedPath} 吗？`)) return;
    await request("/api/fs/file/remove", { method: "DELETE", body: JSON.stringify({ path: selectedPath }) });
    if (currentPath === selectedPath) {
      setCurrentPath("");
      setContent("");
    }
    setSelectedPath("");
    await loadTreeAndBooks();
  }

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        saveFile().catch(console.error);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [content, currentPath]);

  return (
    <div className="app-shell">
      <div className="topbar">
        <strong>Markdown 发布工作台</strong>
        <div>{loading ? "处理中..." : currentPath || "未打开文件"}</div>
      </div>
      <aside className="sidebar">
        <div className="sidebar-actions">
          <button onClick={() => createEntry("file")}>新建文件</button>
          <button onClick={() => createEntry("folder")}>新建目录</button>
          <button onClick={() => renameEntry()} disabled={!selectedPath}>重命名</button>
          <button onClick={() => removeEntry()} disabled={!selectedPath}>删除</button>
        </div>
        <div className="tree"><Tree nodes={tree} currentPath={currentPath} selectedPath={selectedPath} onSelect={setSelectedPath} onOpen={openFile} /></div>
      </aside>
      <section className="main">
        <div className="toolbar">
          <select value={bookDir} onChange={(e) => setBookDir(e.target.value)}>
            {books.map((book) => <option key={book.book_dir_name} value={book.book_dir_name}>{book.book_dir_name}</option>)}
          </select>
          <button onClick={() => saveFile()} disabled={!currentPath}>保存</button>
          <button onClick={() => triggerUpload()} disabled={!bookDir}>上传图片</button>
          <button onClick={() => publish("full")}>完整发布</button>
          <button onClick={() => publish("single")} disabled={!bookDir}>发布当前书籍</button>
          <span className="muted">状态：{publishState}</span>
        </div>
        <div className="editor-wrap">
          <textarea ref={editorRef} value={content} onChange={(e) => setContent(e.target.value)} onPaste={onPaste} placeholder="打开 Markdown 文件后开始编辑" />
          <div className="preview" dangerouslySetInnerHTML={{ __html: marked.parse(content) as string }} />
        </div>
      </section>
      <aside className="panel">
        <h3>发布历史</h3>
        {records.map((record) => (
          <div key={record.record_id} className="record-card">
            <div>{record.publishing_time}</div>
            <div className="muted">{record.publishing_type} | {record.status}</div>
            <button onClick={() => showRecord(record.record_id)}>查看日志</button>
          </div>
        ))}
        <h3>发布日志</h3>
        {publishURL ? <div className="result-url">访问地址：<a href={publishURL} target="_blank" rel="noreferrer">{publishURL}</a></div> : null}
        <div className="log-box">{logs || "暂无日志"}</div>
      </aside>
      <div className="statusbar">
        <span>{bookDir || "未选书籍"}</span>
        <button onClick={async () => {
          await request("/api/logout", { method: "POST" });
          localStorage.removeItem("token");
          location.href = "/login.html";
        }}>退出登录</button>
      </div>
    </div>
  );
}

function Tree({ nodes, currentPath, selectedPath, onSelect, onOpen }: { nodes: TreeNode[]; currentPath: string; selectedPath: string; onSelect: (path: string) => void; onOpen: (path: string) => void }) {
  return (
    <ul>
      {nodes.map((node) => {
        const active = currentPath === node.path;
        const selected = selectedPath === node.path;
        return (
          <li key={node.path}>
            <div className={`tree-item${active ? " active" : ""}${selected ? " selected" : ""}`} onClick={() => onSelect(node.path)}>
              <span>{node.type === "folder" ? "DIR" : "MD"}</span>
              <span className="tree-label" onClick={() => node.type === "file" && onOpen(node.path)}>{node.name}</span>
            </div>
            {node.children && node.children.length > 0 ? <Tree nodes={node.children} currentPath={currentPath} selectedPath={selectedPath} onSelect={onSelect} onOpen={onOpen} /> : null}
          </li>
        );
      })}
    </ul>
  );
}

createRoot(document.getElementById("root")!).render(<App />);
