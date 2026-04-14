# Docker Registry Proxy for EdgeOne Pages

这是一个为 **腾讯 EdgeOne Pages** 单独整理的 Docker Registry 代理仓库。

当前仓库只面向 **EdgeOne Go Functions / Handler Mode**，不再混入 Vercel 结构，从而避免平台识别冲突。

## 保留能力

- 透明代理 Docker Hub（`registry-1.docker.io`）Registry V2 API
- 自动代理 `auth.docker.io` 鉴权，Docker 客户端无需单独登录
- Token 内存缓存，减少重复鉴权
- Docker Hub 官方镜像自动补全 `library/` 前缀
- Blob 下载 CDN 重定向自动跟随
- 多上游仓库路由（`quay.io`、`gcr.io`、`ghcr.io`、`registry.k8s.io` 等）
- 浏览器访问时展示 Docker Hub 镜像搜索页
- 爬虫 UA 屏蔽 + nginx 伪装页
- `/health` 健康检查
- 全部公开路径保持不变：`/`、`/search`、`/v1/*`、`/token`、`/v2/*`、`/health`

## 仓库结构

```text
.
├── cloud-functions/
│   ├── go.mod
│   └── [[path]].go                 # 单一可选 catch-all 路由入口
├── README.md
└── DEPLOYMENT_CHECKLIST.md
```

## 设计说明

基于当前 EdgeOne Builder 的实际行为，最稳妥的做法是：

- 只保留 **一个** 根级路由文件：`cloud-functions/[[path]].go`
- 该文件内包含完整代理逻辑与唯一入口函数 `Handler`
- 不再依赖其他共享 `.go` 文件
- 不再依赖子目录自定义 Go 包

这样可以同时规避两类构建失败：

1. `package ... is not in std`
2. `redeclared in this block`

原因是 EdgeOne 在 `cloud-functions/` 下对 Go 路由文件的编译行为，与标准 Go 多文件包编译并不完全一致。

## 本地验证

由于 `[[path]].go` 是 EdgeOne 的文件路由约定，标准 `go test ./...` / `go build ./...` 无法直接识别该文件名。

建议优先使用：

```bash
edgeone pages build
```

如果需要本地做等效 Go 编译验证，可临时复制：

- `cloud-functions/go.mod`
- `cloud-functions/[[path]].go` → 例如改名为 `main.go`

然后再执行：

```bash
go test ./...
go build ./...
```

## EdgeOne Pages 部署

### 控制台配置

导入仓库后，建议按以下方式填写：

- **Framework Preset**：`Other`
- **Root Directory**：仓库根目录
- **Install Command**：留空
- **Build Command**：留空
- **Output Directory**：留空

### 部署步骤

1. 将 `edgeone-pages` 分支推送到 Git 平台
2. 在 EdgeOne Pages 中导入该分支
3. 保持项目根目录为仓库根目录
4. 保持安装 / 构建 / 输出目录为空
5. 等待 EdgeOne 自动识别 `cloud-functions/[[path]].go`
6. 部署完成后验证：

```bash
curl -i "https://你的域名/v2/"
curl "https://你的域名/health"
curl -I "https://你的域名/"
```

## Docker 客户端使用

### 方式一：配置 `registry-mirrors`

```json
{
  "registry-mirrors": ["https://你的域名"],
  "insecure-registries": []
}
```

### 方式二：直接指定代理地址拉取

```bash
docker pull 你的域名/library/nginx:latest
docker pull 你的域名/library/alpine:latest
```

## 常用验证

### 健康检查

```bash
curl "https://你的域名/health"
```

### Registry V2 探活

```bash
curl -i "https://你的域名/v2/"
```

期望：

- HTTP 200
- Header 含 `Docker-Distribution-Api-Version: registry/2.0`
- Body 为 `{}`

## 支持的上游仓库

| 前缀/参数 | 上游 |
|---|---|
| 默认 | `registry-1.docker.io` |
| `quay` | `quay.io` |
| `gcr` | `gcr.io` |
| `k8s-gcr` | `k8s.gcr.io` |
| `k8s` | `registry.k8s.io` |
| `ghcr` | `ghcr.io` |
| `cloudsmith` | `docker.cloudsmith.io` |
| `nvcr` | `nvcr.io` |

## 参考文档

- EdgeOne Pages Go Runtime：<https://pages.edgeone.ai/zh/document/go>
- EdgeOne Pages Build Guide：<https://pages.edgeone.ai/zh/document/build-guide>
- EdgeOne Pages Go Handler Template：<https://pages.edgeone.ai/templates/go-handler-template>
