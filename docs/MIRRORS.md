# 国内镜像源

## 前后端依赖

- npm：`https://registry.npmmirror.com/`，由 `web/.npmrc` 配置。package-lock.json 的 tarball 地址同样使用该镜像，保留 integrity 校验。
- Go：`https://mirrors.aliyun.com/goproxy/`，由 Makefile 和 Dockerfile 配置，无 direct 回退。校验数据库使用 `sum.golang.google.cn`，保留 go.sum。
- 不修改机器的全局工具配置。手动运行 Go 命令时使用：

```bash
GOPROXY=https://mirrors.aliyun.com/goproxy/ GOSUMDB=sum.golang.google.cn go mod download
```

## Docker 基础镜像

按最新要求采用 DaoCloud 国内代理，无需直接连接 Docker Hub：

| ARG | 默认值 |
| --- | --- |
| NODE_IMAGE | m.daocloud.io/docker.io/library/node:24-alpine |
| GO_IMAGE | m.daocloud.io/docker.io/library/golang:1.26-alpine |
| RUNTIME_IMAGE | m.daocloud.io/docker.io/library/alpine:3.22 |

Dockerfile 与 compose.yaml 默认值一致。Compose 没有将镜像覆盖回 Docker Hub；需要切换自有国内仓库时，可通过 .env 同名变量或 `docker build --build-arg NODE_IMAGE=... --build-arg GO_IMAGE=... --build-arg RUNTIME_IMAGE=...` 覆盖。

```bash
docker pull m.daocloud.io/docker.io/library/node:24-alpine
docker pull m.daocloud.io/docker.io/library/golang:1.26-alpine
docker pull m.daocloud.io/docker.io/library/alpine:3.22
docker build -t interview-project:local .
```

不需要配置阿里云账号专属 Docker 加速地址。若代理或特定标签不可用，保留失败证据后更换经验证的国内仓库，不静默回退到 Docker Hub。
