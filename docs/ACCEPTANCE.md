# MVP 验收记录

验收日期：2026-11-13（Asia/Shanghai）。结论：PRD 第 8 节的 9 条验收标准全部通过。

## 实际验证环境

- macOS arm64：Go 1.26.0、Node.js 24.12.0、Chrome（Playwright 驱动）。
- Docker / Colima：三个基础镜像均实际通过 m.daocloud.io/docker.io/library/ 拉取；镜像构建成功。
- npm 容器安装实际使用 web/.npmrc 和国内源锁文件；Go 容器依赖下载实际使用阿里云 GOPROXY。
- 单个非 root Go 容器托管 Gin API 和 TanStack Start SPA 资源；不运行 Node.js 服务。
- 本地进程、docker run、项目 compose.yaml 均实际完成验收。

## PRD 逐项结果

| 标准 | 结果 | 直接证据 |
| --- | --- | --- |
| 1. 网页上传后列表可见，默认无法公开访问 | 通过 | Chrome 上传 PDF/图片/附件，列表显示未分享；真实 CLI 上传返回 share_url=null；无公开存储路由 |
| 2. 未登录接收者可预览或下载 | 通过 | 独立无登录浏览器上下文打开链接，PDF/图片预览成功；PDF/图片/附件下载内容与原文件逐字节相同；HTTP HEAD 和 Range 验证通过 |
| 3. 撤销后原链接不能发起预览或下载 | 通过 | 原 token 的信息、GET、HEAD、Range 均返回 404；接收页刷新显示失效 |
| 4. 重新分享新链接有效，旧链接仍失效 | 通过 | 新旧 token 不同；新链接可下载，旧链接返回 404；浏览器重复分享流程通过 |
| 5. CLI 独立完成上传、查询、分享、撤销 | 通过 | 使用实际编译的 bin/fileshare 对运行服务执行 config set、upload、list、get、share、revoke、delete，解析 JSON；失败 stdout 为空且非零退出 |
| 6. 按 Skill 分享本地 PDF 并返回链接 | 通过 | 执行 Skill 的 upload PATH --share --json，读取实际 share_url 并访问 PDF；继续执行查询与撤销工作流，验证链接失效；Skill 结构校验通过 |
| 7. 服务重启保留文件和分享状态 | 通过 | 本地进程重启、Docker 容器重启、Compose 服务重启后，内容不变、当前链接不变且有效、已撤销链接仍失效 |
| 8. 未认证请求不能管理文件 | 通过 | 所有 8 个管理路由分别以无凭证和无效 Bearer 请求，均返回 401；另有伪造 Cookie、CSRF 缺失/错误、跨站来源测试 |
| 9. 删除后文件不可下载，链接失效 | 通过 | 实际 CLI 与网页删除后，原链接 404，列表及详情不可见、不能重新分享；业务测试确认磁盘内容删除 |

## 附加关键检查

- 并发开启分享：16 个请求得到同一链接，数据库仅一条未撤销记录。
- 开启分享与删除并发：删除完成后无有效分享残留。
- 同名上传保留不同文件 ID；名称搜索中的 % 和 _ 按字面量处理。
- 超限上传返回 413，临时文件不残留；启动时清理无引用文件。
- 数据库查询确认 shares 无物理外键；跨表事务由服务层维护。
- HTML 等主动内容以附件下载，存储目录和不存在的 API/资源均不可直接访问。
- Cookie 登录/退出、复制链接、搜索空态、删除取消、窄屏布局通过浏览器验证；桌面流程无未处理的页面 JavaScript 异常。
- Compose 实际启动及网页登录通过，确认 bcrypt 哈希未因变量展开损坏，命名卷可持久化。

## 执行命令与结果

| 命令 | 结果 |
| --- | --- |
| go test -race ./... | cli、httpapi、store 测试通过，无竞态报告 |
| go vet ./... | 通过 |
| npm --prefix web run build | 通过，生成静态 _shell.html 与 assets |
| npm --prefix web run typecheck | 通过 |
| skill-creator/scripts/quick_validate.py skills/fileshare | Skill is valid! |
| python3 scripts/acceptance.py --browser | 11 项检查通过，2 个浏览器测试通过 |
| docker build -t interview-project:local . | 通过，三阶段构建 |
| python3 scripts/acceptance.py --docker --browser | 11 项检查通过，2 个浏览器测试通过 |
| python3 scripts/acceptance.py --compose --browser | 11 项检查通过，2 个浏览器测试通过 |

上述命令在项目根目录执行；Go 命令使用 Makefile 中相同的国内源环境。Skill 校验使用 skill-creator 附带的脚本及带 PyYAML 的临时虚拟环境，产品本身不依赖 PyYAML。

可复核的运行产物：work/acceptance-local.json、work/acceptance-docker.json（最终为 Compose 结果）、work/docker-build.log，以及 work/screenshots/ 下的桌面、手机和 PDF 页面截图。运行产物位于忽略目录；验收脚本和测试源码已保存在项目中，可重新生成。

## 交付与范围

已交付 Go + Gin 服务、TanStack Start 管理界面/公开分享页、Go CLI、Agent Skill、Dockerfile/Compose、配置脚本、使用说明和接口/数据库文档。配置脚本由使用者设置账号密码和令牌，不包含默认凭证。

测试环境及专用临时数据卷已清理。没有将产品发布到外部服务器。单机部署由 README 的配置和 Compose 命令完成；撤销无法收回已下载文件或保证中断已开始的传输。
