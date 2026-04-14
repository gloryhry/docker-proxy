# Docker Registry Proxy for EdgeOne Pages

这是一个为 **腾讯 EdgeOne Pages** 单独整理的 Docker Registry 代理仓库。

当前仓库只面向 **EdgeOne Go Runtime / Framework Mode**，不再混入 Vercel 结构，从而避免平台识别冲突。

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
│   ├── index.go                    # EdgeOne 单入口
│   └── internal/proxy/
│       ├── handler.go              # 核心代理逻辑
│       └── handler_test.go         # 核心测试
├── README.md
└── DEPLOYMENT_CHECKLIST.md
```

## 设计说明

基于 EdgeOne Pages 官方 Go 文档，当前仓库采用 **Framework Mode**：

- 运行时代码全部放在 `cloud-functions/`
- `cloud-functions/index.go` 作为唯一 Go 入口
- 所有 HTTP 路由统一交给 `internal/proxy` 中的共享 handler
- `cloud-functions/go.mod` 独立存在，避免根目录混合其他平台配置

这样做的目标是：

- **KISS**：只保留 EdgeOne 需要的结构
- **DRY**：所有 HTTP 逻辑仍集中在一套共享 handler 中
- **YAGNI**：不再保留 Vercel / VPS / 多平台混合入口
- **SOLID**：路由入口与代理核心分离，职责清晰

## 本地验证

在仓库根目录执行：

```bash
cd "cloud-functions"
go test ./...
go build ./...
```

## EdgeOne Pages 部署

### 控制台配置

导入仓库后，建议按以下方式填写：

- **框架（Framework Preset）**：`Other`
- **Root Directory**：仓库根目录
- **Install Command**：留空
- **Build Command**：留空
- **Output Directory**：留空

原因：

- 这是 Go Functions 仓库，不是静态站点
- 运行时代码已按 EdgeOne 约定放在 `cloud-functions/`
- 不需要额外的前端构建步骤

### 部署步骤

1. 将当前分支代码推送到 Git 仓库
2. 在 EdgeOne Pages 中导入该仓库
3. 保持项目根目录为仓库根目录
4. 保持安装/构建/输出目录为空
5. 等待 EdgeOne 自动识别 `cloud-functions/` 下的 Go Functions
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

### Manifest 拉取验证

```bash
curl -s "https://你的域名/v2/library/alpine/manifests/latest" \
  -H "Accept: application/vnd.docker.distribution.manifest.v2+json" | head -20
```

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
