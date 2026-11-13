---
name: fileshare
description: 使用 fileshare CLI 上传并分享本地文件、查询团队已有文件，或撤销文件分享。适用于已部署的 Sharebox 文件分享服务。
---

# Sharebox 文件分享

## 前提

`fileshare` 已加入 PATH，且已配置服务地址和访问令牌。可由环境变量 `FILESHARE_SERVER`、`FILESHARE_TOKEN` 提供，或通过 `fileshare config set --server <origin> --token <token>` 保存配置。环境变量优先于配置文件。不要将令牌写入此 Skill、日志或用户回复。

始终使用 `--json` 读取结构化结果。成功退出码为 0；失败为非零，stderr 给出原因，stdout 不含成功结果。文件时间为 UTC Unix 毫秒，`share_url` 未分享时为 null。

## 分享本地文件

确认用户指定的路径存在且为普通文件，带空格的路径使用引号。执行：

```bash
fileshare upload "/absolute/path/report.pdf" --share --json
```

成功输出示例：

```json
{"file_id":"example-file-id","name":"report.pdf","mime_type":"application/pdf","size_bytes":1024,"created_at":1794560000000,"share_status":"shared","share_url":"https://share.example.com/s/example-token"}
```

将实际返回的 `share_url` 给用户，不自行拼接地址。不要自动打开文件内容或将本地文件发送到其他服务。

若仅需上传保留，不带 `--share`。上传成功但分享失败时，stderr 会给出已经上传的 `file_id`；对该 ID 重试 `fileshare share <file-id> --json`，不要重复上传。网络超时且结果不明确时，先查询确认是否已上传，避免盲目重试产生重复文件。

## 查询已有文件

```bash
fileshare list --search "report" --page 1 --page-size 20 --json
fileshare get <file-id> --json
```

列表返回 `items`、`total`、`page`、`page_size`。按需翻页，不把第一页当成全部结果。展示名称、ID 和分享状态；用户要链接时返回已有的 `share_url`。若文件尚未分享且用户要求分享，执行：

```bash
fileshare share <file-id> --json
```

同名文件可重复存在。涉及分享、撤销或删除且有多个候选时，请用户确认目标 ID，不任意选择。

## 停止分享

先通过明确的文件 ID 或查询结果确定目标，再执行：

```bash
fileshare revoke <file-id> --json
```

成功后告知用户原链接已失效，文件仍保留。重复撤销也视为成功。再次分享会生成新链接，不能恢复旧链接。撤销无法收回接收者已经下载的内容。

只有用户明确要求删除文件时才用 `fileshare delete <file-id> --json`；停止分享使用 revoke。

## 错误处理

- 本地路径不存在：说明路径问题，等待正确路径。
- 401：提示检查访问令牌配置，不索取或回显已有令牌。
- 413：说明文件超出服务大小限制（默认 100 MB）。
- 404：重新查询目标；不要把失效链接当作可访问内容返回。
- 网络错误：检查服务地址与连通性；可通过 `--timeout 5m` 调整大文件请求超时，禁止无限重试。
