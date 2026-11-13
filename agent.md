# 项目开发约束

## 数据库关联

- 表之间只能使用逻辑外键：保留关联 ID 字段和必要索引，在设计文档中明确关联关系。
- 禁止在建表、迁移和 ORM 模型中创建物理外键，包括 FOREIGN KEY、REFERENCES 及数据库级联更新或删除。
- 禁止通过跨表触发器实现关联校验或级联操作。
- 关联对象存在性、状态校验和关联数据变更由应用服务层负责；需要原子性的操作必须使用数据库事务。
- 例如：创建分享时检查文件存在且未删除；删除文件时，在同一事务中标记文件删除并撤销有效分享。
- 主键、唯一索引、非空和单表 CHECK 约束可以正常使用。
- E-R 图中的 FK 仅表示逻辑关联，不代表数据库物理外键。

## 依赖与容器镜像源

- 前端 npm 依赖使用 https://registry.npmmirror.com/，锁文件下载地址也必须指向国内镜像，保留完整性校验。
- Go 依赖使用 https://mirrors.aliyun.com/goproxy/，保留 go.sum 校验；不默认回退到境外源。
- Docker 默认通过 m.daocloud.io/docker.io/library/ 拉取 node、golang 和 alpine，不直接连接 Docker Hub；保留 ARG 覆盖机制。
- 不修改开发者机器的全局 npm、Go 或 Docker 配置，源配置保存在项目中。
