# HTTP API

同源部署。管理接口支持 `Authorization: Bearer <token>` 或网页登录 Cookie。Cookie 写请求须带 `/api/session` 返回的 `X-CSRF-Token`；有 Origin 时必须与 FILESHARE_PUBLIC_URL 一致。登录 Cookie 有效期 12 小时，HttpOnly、SameSite=Strict，HTTPS 下 Secure。CLI 不使用 Cookie。

## 路由

| 方法 | 路由 | 请求 / 结果 |
| --- | --- | --- |
| POST | /api/login | JSON username、password；返回 username、csrf_token、max_upload_bytes，并设置 Cookie |
| GET | /api/session | 返回 username、csrf_token、max_upload_bytes |
| POST | /api/logout | 清除 Cookie，返回 `{ "ok": true }` |
| POST | /api/files | multipart/form-data，仅一个 file 字段；201 返回文件 |
| GET | /api/files | search 为名称字面量包含搜索；page 默认 1，page_size 默认 20、最大 100；返回列表 |
| GET | /api/files/:id | 返回文件及当前分享链接 |
| POST | /api/files/:id/share | 幂等开启分享，返回文件 |
| DELETE | /api/files/:id/share | 幂等撤销分享，返回未分享文件 |
| DELETE | /api/files/:id | 软删除元数据、撤销分享并清理磁盘；返回 file_id、deleted=true；重复删除成功 |
| GET | /api/public/shares/:token | 匿名文件信息，share_url 为 null |
| GET/HEAD | /api/public/shares/:token/content | 匿名文件内容，支持 Range；download=1 强制附件下载 |
| GET | /s/:token | 分享页面静态外壳，页面通过公开 API 检查有效性 |
| GET | /healthz | 进程存活状态 |

## 数据格式

文件：

```json
{"file_id":"uuid","name":"report.pdf","mime_type":"application/pdf","size_bytes":1024,"created_at":1794560000000,"share_status":"shared","share_url":"https://share.example.com/s/random-token"}
```

未分享时 share_status=unshared，share_url=null。时间为 UTC Unix 毫秒，字节数为实际文件大小。文件 ID 和存储键由服务端生成；内部存储键不返回给客户端。

列表：

```json
{"items":[],"total":0,"page":1,"page_size":20}
```

按 created_at DESC、file_id DESC 排序。偏移分页在不同请求间遇到新上传时可能移动，客户端刷新列表即可；不承诺跨请求快照。

错误：

```json
{"code":"not_found","message":"文件不存在或分享已失效"}
```

| HTTP | code | 含义 |
| --- | --- | --- |
| 400 | invalid_request | 参数或上传格式错误 |
| 401 | unauthorized | 未登录或令牌无效 |
| 403 | forbidden | CSRF 或来源校验失败 |
| 404 | not_found | 文件不存在、已删除、分享撤销或资源不存在 |
| 413 | file_too_large | 超出上传限制 |
| 500 | internal_error | 内部错误，详细凭证不返回客户端 |
| 503 | frontend_unavailable | 尚未构建前端 |

所有文件和分享响应为 Cache-Control: no-store。仅 PDF、PNG、JPEG、GIF、WebP 可内联预览，其余按附件下载。每个信息/内容/Range 请求均重新校验分享状态。撤销不保证中断正在进行的传输。公开分享外壳本身不含文件信息，撤销后页面展示失效提示。
