import {
  createRootRoute,
  HeadContent,
  Outlet,
  Scripts,
} from "@tanstack/react-router";
import type { ReactNode } from "react";
import stylesheet from "../styles.css?url";

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: "Sharebox · 团队文件分享" },
    ],
    links: [{ rel: "stylesheet", href: stylesheet }],
  }),
  shellComponent: Shell,
  component: Outlet,
  notFoundComponent: () => (
    <main className="page-loading">
      <h1>页面不存在</h1>
      <a href="/">返回文件管理</a>
    </main>
  ),
  errorComponent: () => (
    <main className="page-loading">
      <h1>页面加载失败</h1>
      <button onClick={() => window.location.reload()}>重新加载</button>
    </main>
  ),
});
function Shell({ children }: { children: ReactNode }) {
  return (
    <html lang="zh-CN">
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  );
}
