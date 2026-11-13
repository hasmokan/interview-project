import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  api,
  getSession,
  previewable,
  sizeLabel,
  type FilePage,
  type Session,
  type SharedFile,
} from "./api";

type IconName =
  | "box"
  | "upload"
  | "search"
  | "file"
  | "link"
  | "check"
  | "arrow"
  | "trash"
  | "image"
  | "download"
  | "terminal"
  | "stop"
  | "logout";
const paths: Record<IconName, ReactNode> = {
  box: (
    <>
      <path d="m12 3 9 5v8l-9 5-9-5V8l9-5Z" />
      <path d="m3 8 9 5 9-5M12 13v8M7.5 5.5l9 5" />
    </>
  ),
  upload: (
    <>
      <path d="M12 16V3m-5 5 5-5 5 5M4 15v5h16v-5" />
    </>
  ),
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m16 16 5 5" />
    </>
  ),
  file: (
    <>
      <path d="M14 3H5v18h14V8l-5-5Z" />
      <path d="M14 3v6h5M8 13h8M8 17h5" />
    </>
  ),
  link: (
    <>
      <path
        d="m10 13 4-4m-6 6-2 2a4 4 0 0 1-5-6l4-4a4 4 0 0 1 6 0m2 2 2-2a4 4 0 0 1 6 6l-4 4a4 4 0 0 1-6 0"
        transform="translate(1 0)"
      />
    </>
  ),
  check: <path d="m5 12 4 4L19 6" />,
  arrow: <path d="M4 12h16m-6-6 6 6-6 6" />,
  trash: (
    <>
      <path d="M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7" />
    </>
  ),
  image: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="3" />
      <circle cx="8" cy="8" r="1" />
      <path d="m3 17 6-6 4 4 3-3 5 5" />
    </>
  ),
  download: (
    <>
      <path d="M12 3v13m-5-5 5 5 5-5M4 17v4h16v-4" />
    </>
  ),
  terminal: (
    <>
      <rect x="2" y="4" width="20" height="16" rx="3" />
      <path d="m6 9 3 3-3 3m6 0h5" />
    </>
  ),
  stop: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m6 6 12 12" />
    </>
  ),
  logout: (
    <>
      <path d="M10 4H4v16h6m3-13 5 5-5 5m-4-5h12" />
    </>
  ),
};
function Icon({ name, size = 20 }: { name: IconName; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  );
}
function Brand() {
  return (
    <a className="brand" href="/" aria-label="Sharebox 首页">
      <span className="brand-symbol">
        <Icon name="box" size={24} />
      </span>
      Sharebox<span className="brand-label">团队分享</span>
    </a>
  );
}
function Header({ children }: { children?: ReactNode }) {
  return (
    <header className="header">
      <div className="header-inner">
        <Brand />
        {children}
      </div>
    </header>
  );
}
function FileGlyph({ file }: { file: SharedFile }) {
  const type =
    file.mime_type === "application/pdf"
      ? "pdf"
      : file.mime_type.startsWith("image/")
        ? "img"
        : "doc";
  return (
    <span className={`file-glyph ${type}`}>
      <Icon name={type === "img" ? "image" : "file"} size={23} />
    </span>
  );
}
function message(error: unknown) {
  return error instanceof Error ? error.message : "网络连接失败，请重试";
}

export function Login() {
  const navigate = useNavigate();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(Object.fromEntries(data)),
      });
      await navigate({ to: "/" });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Header />
      <main className="login-layout">
        <section className="login-intro">
          <span className="eyebrow">A SMALL SPACE FOR SHARING</span>
          <h1>
            让成果，
            <br />
            轻松抵达。
          </h1>
          <p>
            报告、图片，或 AI 刚完成的作品。
            <br />
            上传一份文件，分享一个链接，随时收回访问。
          </p>
          <div className="intro-note">
            <Icon name="link" />
            简单分享 · 自主管理
          </div>
        </section>
        <section className="login-card">
          <span className="eyebrow">WORKSPACE</span>
          <h2>登录团队空间</h2>
          <p className="muted">使用团队管理账号继续</p>
          <form onSubmit={login}>
            <label>
              账号
              <input
                name="username"
                autoComplete="username"
                placeholder="输入管理账号"
                required
              />
            </label>
            <label>
              密码
              <input
                name="password"
                type="password"
                autoComplete="current-password"
                placeholder="输入密码"
                required
              />
            </label>
            {error ? (
              <p className="error" role="alert">
                {error}
              </p>
            ) : null}
            <button className="primary full" disabled={busy}>
              {busy ? "正在登录…" : "进入文件空间"}
              <Icon name="arrow" />
            </button>
          </form>
          <p className="login-foot">接收分享的文件？直接打开分享链接即可。</p>
        </section>
      </main>
      <Footer />
    </>
  );
}
function Footer() {
  return (
    <footer className="footer">
      <span>Sharebox</span>
      <span>每一份成果，都值得被看见。</span>
    </footer>
  );
}
export function Dashboard() {
  const [session, setSession] = useState<Session | null>(null);
  const [data, setData] = useState<FilePage | null>(null);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [refresh, setRefresh] = useState(0);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [busy, setBusy] = useState("");
  const [dragging, setDragging] = useState(false);
  const [deleting, setDeleting] = useState<SharedFile | null>(null);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    let live = true;
    getSession()
      .then((s) => {
        if (live) setSession(s);
      })
      .catch((e) => {
        if (live) setError(message(e));
      });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    const timer = setTimeout(() => {
      setQuery(search);
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [search]);
  useEffect(() => {
    if (!session) return;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    api<FilePage>(
      `/api/files?search=${encodeURIComponent(query)}&page=${page}`,
      { signal: controller.signal },
    )
      .then((d) => {
        if (d.items.length === 0 && page > 1) {
          setPage((p) => p - 1);
          return;
        }
        setData(d);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [session, query, page, refresh]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 4500);
    return () => clearTimeout(t);
  }, [notice]);
  async function upload(files: FileList | null) {
    if (!files?.length || uploading) return;
    setError("");
    if (files.length !== 1) {
      setError("每次请选择一个文件上传");
      return;
    }
    if (session && files[0].size > session.max_upload_bytes) {
      setError(`文件不能超过 ${sizeLabel(session.max_upload_bytes)}`);
      return;
    }
    setUploading(true);
    const form = new FormData();
    form.set("file", files[0]);
    try {
      await api("/api/files", { method: "POST", body: form });
      setNotice("上传成功，文件尚未分享");
      setSearch("");
      setQuery("");
      setPage(1);
      setRefresh((n) => n + 1);
    } catch (e) {
      setError(message(e));
    } finally {
      setUploading(false);
      if (input.current) input.current.value = "";
    }
  }
  async function action(
    file: SharedFile,
    op: "share" | "revoke" | "delete" | "copy",
  ) {
    setBusy(file.file_id);
    setError("");
    try {
      if (op === "copy") {
        await navigator.clipboard.writeText(file.share_url!);
        setNotice("分享链接已复制");
      } else {
        await api(
          `/api/files/${file.file_id}${op === "delete" ? "" : "/share"}`,
          { method: op === "share" ? "POST" : "DELETE" },
        );
        setNotice(
          op === "share"
            ? "分享已开启，点击“复制链接”即可发送"
            : op === "revoke"
              ? "分享已撤销，原链接已失效"
              : "文件已删除，分享链接已失效",
        );
        setRefresh((n) => n + 1);
        setDeleting(null);
      }
    } catch (e) {
      setError(
        op === "copy" ? "复制失败，请打开文件链接后从地址栏复制" : message(e),
      );
    } finally {
      setBusy("");
    }
  }
  return (
    <>
      <Header>
        <div className="account">
          <span className="avatar">
            {session?.username.slice(0, 1).toUpperCase() || "·"}
          </span>
          <span>{session?.username || "团队空间"}</span>
          <button
            className="icon-button"
            aria-label="退出登录"
            onClick={async () => {
              try {
                await api("/api/logout", { method: "POST" });
                window.location.assign("/login");
              } catch (e) {
                setError(message(e));
              }
            }}
          >
            <Icon name="logout" size={18} />
          </button>
        </div>
      </Header>
      <main className="workspace">
        <section className="page-title">
          <div>
            <span className="eyebrow">YOUR TEAM WORKSPACE</span>
            <h1>
              文件空间<span className="title-dot">.</span>
            </h1>
            <p className="muted">把文件留在这里，把链接分享出去。</p>
          </div>
          <span className="private-note">
            <span className="status-dot" />
            上传后默认仅团队可见
          </span>
        </section>
        <section
          className={`upload-zone ${dragging ? "dragging" : ""} ${uploading ? "uploading" : ""}`}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            void upload(e.dataTransfer.files);
          }}
          aria-label="上传文件区域"
        >
          <div className="upload-art">
            <Icon name="upload" size={27} />
          </div>
          <div className="upload-copy">
            <h2>{uploading ? "正在上传文件…" : "新的成果，从这里分享"}</h2>
            <p>
              拖放一个文件到这里，或点击右侧选择文件。
              <span>
                单文件最大{" "}
                {sizeLabel(session?.max_upload_bytes || 100 * 1024 * 1024)}
              </span>
            </p>
          </div>
          <input
            ref={input}
            className="visually-hidden"
            type="file"
            tabIndex={-1}
            aria-label="选择上传文件"
            onChange={(e) => void upload(e.target.files)}
          />
          <button
            className="primary"
            disabled={!session || uploading}
            onClick={() => input.current?.click()}
          >
            <Icon name="upload" size={17} />
            {uploading ? "上传中…" : "上传文件"}
          </button>
        </section>
        <div aria-live="polite">
          {notice ? (
            <div className="notice">
              <Icon name="check" size={17} />
              {notice}
            </div>
          ) : null}
        </div>
        {error ? (
          <div className="error error-banner" role="alert">
            {error}
            <button onClick={() => setRefresh((n) => n + 1)}>重试</button>
          </div>
        ) : null}
        <section className="file-panel">
          <div className="panel-toolbar">
            <div className="panel-title">
              <h2>全部文件</h2>
              <span className="count">{data?.total ?? "—"}</span>
            </div>
            <label className="search">
              <Icon name="search" size={18} />
              <input
                aria-label="搜索文件"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="搜索文件名称…"
              />
            </label>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>文件名称</th>
                  <th>大小</th>
                  <th>上传时间</th>
                  <th>分享状态</th>
                  <th className="actions-head">操作</th>
                </tr>
              </thead>
              <tbody>
                {data?.items.map((file) => (
                  <tr key={file.file_id}>
                    <td>
                      <div className="file-name">
                        <FileGlyph file={file} />
                        <div>
                          <span className="filename" title={file.name}>
                            {file.name}
                          </span>
                          <span className="file-type">
                            {file.mime_type === "application/pdf"
                              ? "PDF 文档"
                              : file.mime_type.startsWith("image/")
                                ? "图片"
                                : "文件"}
                          </span>
                        </div>
                      </div>
                    </td>
                    <td className="meta-cell">{sizeLabel(file.size_bytes)}</td>
                    <td className="meta-cell">
                      {new Date(file.created_at).toLocaleString("zh-CN", {
                        month: "2-digit",
                        day: "2-digit",
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </td>
                    <td>
                      <span className={`badge ${file.share_status}`}>
                        <span className="status-dot" />
                        {file.share_status === "shared" ? "分享中" : "未分享"}
                      </span>
                    </td>
                    <td>
                      <div className="row-actions">
                        {file.share_status === "shared" ? (
                          <>
                            <button
                              disabled={busy === file.file_id}
                              className="text-button green"
                              onClick={() => void action(file, "copy")}
                            >
                              <Icon name="link" size={15} />
                              复制链接
                            </button>
                            <button
                              disabled={busy === file.file_id}
                              className="text-button"
                              onClick={() => void action(file, "revoke")}
                            >
                              撤销
                            </button>
                          </>
                        ) : (
                          <button
                            className="text-button green"
                            disabled={busy === file.file_id}
                            onClick={() => void action(file, "share")}
                          >
                            <Icon name="link" size={15} />
                            开启分享
                          </button>
                        )}
                        <button
                          className="icon-button delete-button"
                          disabled={busy === file.file_id}
                          aria-label={`删除 ${file.name}`}
                          onClick={() => setDeleting(file)}
                        >
                          <Icon name="trash" size={17} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {loading ? (
            <div className="list-state" role="status">
              正在加载文件…
            </div>
          ) : !data?.items.length ? (
            <div className="empty-state">
              <span className="empty-icon">
                <Icon name={query ? "search" : "box"} size={32} />
              </span>
              <h3>
                {query ? "没有找到匹配的文件" : "空间已就绪，上传第一份文件吧"}
              </h3>
              <p>
                {query
                  ? "试试其他名称，或清空搜索。"
                  : "支持报告、图片和其他文件，上传后由你决定何时分享。"}
              </p>
              {query ? (
                <button
                  className="text-button green"
                  onClick={() => setSearch("")}
                >
                  清空搜索
                </button>
              ) : null}
            </div>
          ) : null}
          <div className="panel-bottom">
            <span>
              {query ? "搜索结果" : "文件保存在团队服务器"}
              {loading ? " · 更新中" : ""}
            </span>
            <div className="pagination">
              <button
                aria-label="上一页"
                disabled={page <= 1 || loading}
                onClick={() => setPage((p) => p - 1)}
              >
                ←
              </button>
              <span>
                {page} / {Math.max(1, Math.ceil((data?.total || 0) / 20))}
              </span>
              <button
                aria-label="下一页"
                disabled={!data || page * 20 >= data.total || loading}
                onClick={() => setPage((p) => p + 1)}
              >
                →
              </button>
            </div>
          </div>
        </section>
        <aside className="agent-note">
          <span className="terminal-icon">
            <Icon name="terminal" size={23} />
          </span>
          <div>
            <strong>也为你的 AI Agent 准备好了</strong>
            <p>通过 CLI 和 Skill，让 Agent 上传文件、返回链接，或停止分享。</p>
          </div>
          <code>fileshare upload ./report.pdf --share --json</code>
        </aside>
      </main>
      <Footer />
      {deleting ? (
        <div
          className="modal-backdrop"
          onClick={() => {
            if (!busy) setDeleting(null);
          }}
        >
          <section
            className="modal"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-title"
          >
            <span className="modal-icon">
              <Icon name="trash" size={24} />
            </span>
            <h2 id="delete-title">删除这个文件？</h2>
            <p className="delete-name">{deleting.name}</p>
            <p className="muted">
              文件将被永久删除。
              {deleting.share_status === "shared"
                ? "正在使用的分享链接也会立即失效。"
                : "此操作无法撤销。"}
            </p>
            <div className="modal-actions">
              <button
                autoFocus
                className="secondary"
                disabled={!!busy}
                onClick={() => setDeleting(null)}
              >
                取消
              </button>
              <button
                className="danger"
                disabled={!!busy}
                onClick={() => void action(deleting, "delete")}
              >
                {busy ? "正在删除…" : "确认删除"}
              </button>
            </div>
          </section>
        </div>
      ) : null}
    </>
  );
}

export function SharePage({ token }: { token: string }) {
  const [file, setFile] = useState<SharedFile | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    const load = () =>
      api<SharedFile>(`/api/public/shares/${encodeURIComponent(token)}`, {
        signal: controller.signal,
      })
        .then((f) => {
          setFile(f);
          setError("");
        })
        .catch((e) => {
          if (!controller.signal.aborted) {
            setFile(null);
            setError(message(e));
          }
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    void load();
    const onFocus = () => {
      void load();
    };
    window.addEventListener("focus", onFocus);
    return () => {
      controller.abort();
      window.removeEventListener("focus", onFocus);
    };
  }, [token]);
  const content = `/api/public/shares/${encodeURIComponent(token)}/content`;
  return (
    <>
      <Header>
        <span className="header-caption">一份来自团队的分享</span>
      </Header>
      <main className="share-main">
        {loading ? (
          <div className="page-loading" role="status">
            正在打开文件…
          </div>
        ) : error || !file ? (
          <section className="share-unavailable">
            <span className="empty-icon">
              <Icon name="stop" size={34} />
            </span>
            <h1>分享不存在或已失效</h1>
            <p className="muted">请联系发送者，获取新的分享链接。</p>
            <button
              className="secondary"
              onClick={() => window.location.reload()}
            >
              重新检查
            </button>
          </section>
        ) : (
          <>
            <div className="share-heading">
              <div className="shared-label">
                <span className="status-dot" />
                已与你分享
              </div>
              <h1>{file.name}</h1>
              <p className="muted">
                {sizeLabel(file.size_bytes)}
                <span className="separator">/</span>
                {file.mime_type}
              </p>
            </div>
            <section className="preview-panel">
              {file.mime_type === "application/pdf" ? (
                <iframe
                  title={`${file.name} PDF 预览`}
                  src={content}
                  className="pdf-preview"
                />
              ) : previewable(file.mime_type) ? (
                <img className="image-preview" src={content} alt={file.name} />
              ) : (
                <div className="attachment-preview">
                  <FileGlyph file={file} />
                  <h2>文件已准备好</h2>
                  <p className="muted">
                    此类型暂不支持在线预览，请下载后查看。
                  </p>
                </div>
              )}
              <div className="preview-footer">
                <span>
                  <Icon name="file" size={18} />
                  {file.name}
                </span>
                <a className="primary" href={`${content}?download=1`}>
                  <Icon name="download" size={18} />
                  下载文件
                </a>
              </div>
            </section>
            <p className="share-note">
              通过 Sharebox 安全分享 · 发送者可随时撤销访问
            </p>
          </>
        )}
      </main>
      <Footer />
    </>
  );
}
