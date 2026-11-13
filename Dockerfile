ARG NODE_IMAGE=m.daocloud.io/docker.io/library/node:24-alpine
ARG GO_IMAGE=m.daocloud.io/docker.io/library/golang:1.26-alpine
ARG RUNTIME_IMAGE=m.daocloud.io/docker.io/library/alpine:3.22
FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package*.json web/.npmrc ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build && npm run typecheck

FROM ${GO_IMAGE} AS go
WORKDIR /src
ARG GOPROXY=https://mirrors.aliyun.com/goproxy/
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server && CGO_ENABLED=0 go build -trimpath -o /out/fileshare ./cmd/fileshare

FROM ${RUNTIME_IMAGE}
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app && mkdir -p /data && chown app:app /data
WORKDIR /app
COPY --from=go /out/server /usr/local/bin/server
COPY --from=go /out/fileshare /usr/local/bin/fileshare
COPY --from=web /src/web/dist/client ./web
ENV FILESHARE_ADDR=:8080 FILESHARE_DATA_DIR=/data FILESHARE_WEB_DIR=/app/web
USER app
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["server"]
