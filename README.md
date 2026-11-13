# Sharebox

一个可单机部署的团队文件分享工具：网页上传、搜索、开启分享、复制链接、撤销和删除；接收者匿名预览图片/PDF或下载文件。Go CLI 与 Agent Skill 使用同一套管理 API。

技术栈：Go 1.26 + Gin、TanStack Start + React + TypeScript（SPA）、SQLite、本地文件目录。数据库仅使用逻辑外键。生产环境是单个 Go 容器，不需要 Node.js 运行服务。

## 交付内容

| 交付项 | 入口 |
| --- | --- |
| 后端源码 | [cmd/server](cmd/server)、[internal](internal) |
| 前端源码 | [web](web) |
| Go CLI 源码 | [cmd/fileshare](cmd/fileshare)、[internal/cli](internal/cli) |
| 启动说明 | 本文的“本地构建与启动”和“Docker 单服务部署” |
| Agent Skill | [skills/fileshare/SKILL.md](skills/fileshare/SKILL.md) |
| PRD 与实施计划 | [docs/PRD.md](docs/PRD.md)、[docs/PLAN.md](docs/PLAN.md) |
| 验收记录 | [docs/ACCEPTANCE.md](docs/ACCEPTANCE.md) |

## 获取源码

```bash
git clone https://github.com/hasmokan/interview-project.git
cd interview-project
```

## 本地构建与启动

需要 Go 1.26、Node.js 24、npm，以及 Python 3（用于配置和验收脚本）。

```bash
make build
python3 scripts/setup-env.py
set -a
. ./.env
set +a
./bin/server
```

浏览器打开 http://localhost:8080，用配置时输入的管理账号密码登录。没有内置默认密码或令牌。配置脚本生成 `.env`（权限 0600，已忽略），只显示一次 CLI 令牌，请保存到密钥管理工具。

默认数据目录 `./data`，其中 `fileshare.db` 保存元数据，`files/` 保存内容。数据目录只供一个服务进程使用。

## Docker 单服务部署

镜像默认从 **DaoCloud 国内代理**拉取，npm 与 Go 依赖使用阿里系国内镜像；详见 docs/MIRRORS.md。

在完成上述配置后：

```bash
docker compose up -d --build
docker compose logs -f fileshare
```

Compose 将数据库与文件保存在命名卷 `fileshare-data` 中，普通重启和容器重建保留数据。不要执行 `docker compose down -v`，除非确实要清空数据。容器以非 root 用户运行；使用绑定目录时需保证 UID 10001 可读写。

通过服务器域名使用时，将 `.env` 的 `FILESHARE_PUBLIC_URL` 设置为实际访问的 origin，例如 `https://share.example.com`。公开地址参与链接生成、Cookie Secure 和来源检查。TLS 可由服务器已有的反向代理终止；不要将原始文件目录作为静态目录公开。

基础镜像可用 `NODE_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE` 覆盖，Dockerfile ARG 和 Compose 配置保持一致。默认无需直接连接 Docker Hub。

### 服务配置

| 变量 | 默认 / 说明 |
| --- | --- |
| FILESHARE_ADDR | :8080 |
| FILESHARE_PUBLIC_URL | http://localhost:8080，实际浏览器访问地址，无子路径 |
| FILESHARE_ADMIN_USER | admin |
| FILESHARE_PASSWORD_HASH | 必填，bcrypt 密码哈希 |
| FILESHARE_TOKEN_HASH | 必填，CLI 令牌的 SHA-256 十六进制摘要 |
| FILESHARE_SESSION_KEY | 必填，至少 32 字符的会话签名密钥 |
| FILESHARE_MAX_UPLOAD_BYTES | 104857600（100 MiB） |
| FILESHARE_DATA_DIR | 本地 ./data；容器 /data |
| FILESHARE_WEB_DIR | 本地 ./web/dist/client；容器 /app/web |
| FILESHARE_PORT | Compose 暴露端口，默认 8080 |

`.env` 中的 bcrypt 哈希必须保留单引号，避免 `$` 被 Compose 或 shell 展开。服务读取环境变量，不自动加载 `.env`；Compose 会读取，直接运行时按上面的命令加载。

## CLI

```bash
go build -o bin/fileshare ./cmd/fileshare
# 将 bin/fileshare 放到 PATH，或直接使用 ./bin/fileshare
```

CLI 配置支持环境变量或本地配置文件：

```bash
export FILESHARE_SERVER=http://localhost:8080
# 从密钥管理器或安全输入读取 FILESHARE_TOKEN，不写进脚本
read -r -s FILESHARE_TOKEN
export FILESHARE_TOKEN

./bin/fileshare upload ./report.pdf --share --json
./bin/fileshare list --search report --page 1 --page-size 20 --json
./bin/fileshare get <file-id> --json
./bin/fileshare share <file-id> --json
./bin/fileshare revoke <file-id> --json
./bin/fileshare delete <file-id> --json
```

也可以用 `fileshare config set --server <origin> --token <token>` 保存配置；若使用 shell，避免把实际令牌字面量写进历史。配置文件位于系统用户配置目录的 `fileshare/config.json`（macOS 为 `~/Library/Application Support/fileshare/config.json`），权限 0600；可通过 `FILESHARE_CONFIG` 指定其他位置。环境变量优先于配置文件。

`--json` 保证 stdout 为单个 JSON 结果。失败时非零退出码，错误写 stderr。请求默认超时 2 分钟，可用 `--timeout 5m` 覆盖。上传和分享是两个请求；上传成功但分享失败时，错误信息包含已上传的文件 ID，可直接重试 share，避免重复上传。

## Agent Skill

Skill 位于 `skills/fileshare/SKILL.md`。将 `skills/fileshare` 目录复制到 Agent 的 Skill 搜索目录，并确保 CLI 在 PATH 中。Skill 覆盖：

- “把本地 report.pdf 分享出去，把链接给我。”
- “查询之前上传的报告。”
- “停止这个文件的分享。”

同名文件允许存在，必须使用 ID 确定操作目标；有多个候选时先确认。Skill 不包含服务令牌。

## 前端开发

后端保持运行，前端通过 Vite 代理 `/api`：

```bash
# 后端 FILESHARE_PUBLIC_URL 设置为浏览器使用的前端 origin：http://localhost:5173
cd web
npm ci
npm run dev
```

生产构建使用 `npm run build`，生成 `web/dist/client/_shell.html` 和 assets，Go 托管这些文件。TanStack Start 构建过程生成的服务端代码只用于预渲染，不进入生产运行路径。

## 检查与验收

```bash
make test
npm --prefix web run typecheck
npm --prefix web run build
python3 scripts/acceptance.py --browser
docker build -t interview-project:local .
python3 scripts/acceptance.py --compose --browser
```

验收脚本要求 `bin/server`、`bin/fileshare` 和前端已构建。脚本使用随机凭证、临时数据目录/独立 Docker Volume 和空闲端口，结束后自动清理，不影响实际部署数据。浏览器测试在 macOS 自动使用已安装的 Chrome；其他系统可设置 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指向 Chrome/Chromium，或安装 Playwright 所需的 Chromium。无需在运行容器中安装浏览器。

验收包含真实 CLI 上传与分享、匿名 GET/HEAD/Range 下载、撤销及重新分享、服务重启持久性、全部管理路由鉴权、删除后失效；浏览器验证 PDF/图片/附件、复制、搜索、删除确认、移动布局和退出。

结果摘要写入 work/acceptance-local.json 和 work/acceptance-docker.json，浏览器截图写入 work/screenshots/。最终 PRD 验收记录见 docs/ACCEPTANCE.md。

## 设计文档

- docs/PRD.md：需求与 9 条验收标准。
- docs/PLAN.md：实施阶段与完成情况。
- docs/API.md：接口约定。
- docs/DATABASE.md、docs/schema.sql：数据库设计与 E-R 图。
- docs/MIRRORS.md：国内源与镜像覆盖方式。

上传默认不分享。撤销后重新分享会生成新链接，旧链接永久失效；删除会同时撤销分享。撤销不能收回已下载内容或保证中止已开始的传输。
