# Docker Registry Proxy

自建 Docker Registry 代理服务，现已重构为 **Go 标准库共享核心 + 平台适配层**，可直接部署到：

- **腾讯 EdgeOne Pages（Go Runtime）**
- **Vercel（Go Serverless Function + Rewrite）**

项目完整保留原有能力：

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

> 当前版本**不再提供 VPS 二进制常驻进程 / TLS / 守护进程模式**。

## 项目结构

```text
.
├── pkg/proxy/handler.go     # 共享核心：统一 http.Handler
├── api/index.go             # Vercel 入口
├── cloud-functions/         # EdgeOne Pages Go Runtime 入口
│   ├── go.mod
│   ├── index.go
│   ├── [[path]].go
│   └── shared/handler.go
├── vercel.json              # Vercel rewrite 配置
└── pkg/proxy/handler_test.go # 共享核心测试
```

## 本地验证

### 根模块测试

```bash
go test ./...
```

### EdgeOne Functions 模块测试/编译

```bash
cd "cloud-functions"
go test ./...
go build ./...
```

## Docker 客户端使用方式

部署成功后，直接把你的 Pages/Vercel 域名配置为镜像代理地址即可。

### 方式一：配置 `registry-mirrors`

编辑 `/etc/docker/daemon.json`：

```json
{
  "registry-mirrors": ["https://你的域名"],
  "insecure-registries": []
}
```

重启 Docker：

```bash
sudo systemctl daemon-reload
sudo systemctl restart docker
```

之后即可直接：

```bash
docker pull nginx
docker pull ubuntu:22.04
```

### 方式二：直接指定代理地址拉取

```bash
docker pull 你的域名/library/nginx:latest
docker pull 你的域名/bitnami/redis:latest
```

## EdgeOne Pages 部署

根据 EdgeOne 官方 Go Runtime 文档，当前项目使用 **Framework 模式 + 标准库 `net/http` 服务**：

- 入口文件为 `cloud-functions/index.go`
- 入口文件名为 `index.go`，因此外部访问**无额外路径前缀**
- 为兼容 EdgeOne Builder 的 Handler Mode 文件合并行为，`cloud-functions/shared/handler.go` 提供共享实现，路由文件仅保留薄包装

### 步骤

1. 将仓库推送到 Git 平台
2. 在 EdgeOne Pages 中导入该仓库
3. 保持 Pages 根目录为仓库根目录
4. 平台会自动识别 `cloud-functions/index.go` 并构建 Go 运行时服务
5. 部署完成后，直接使用 Pages 分配域名访问

### 路由说明

- 所有外部路径都会进入 `cloud-functions/index.go` 中启动的 Go HTTP 服务
- 服务内部继续由共享核心路由处理：
  - `/`
  - `/search`
  - `/v1/*`
  - `/token`
  - `/v2/*`
  - `/health`

## Vercel 部署

根据 Vercel 官方 Go Runtime 文档，当前项目使用：

- 根目录 `go.mod`
- `/api/index.go` 单函数入口
- `vercel.json` rewrite，将全部公开路径转发到该函数

### 步骤

1. 将仓库导入 Vercel
2. 不需要改代码入口
3. 保持项目根目录为仓库根目录
4. 平台会自动识别根目录 `go.mod` 与 `/api/index.go`
5. 部署完成后，直接使用 Vercel 域名访问

### 路由说明

用户访问路径不会变，仍然是：

- `/`
- `/search`
- `/v1/*`
- `/token`
- `/v2/*`
- `/health`

Vercel 通过 `vercel.json` 在内部将这些路径 rewrite 到 `/api/index.go`，然后再恢复原始路径交给共享核心处理。

## 诊断

### 健康检查

```bash
curl "https://你的域名/health"
```

返回示例：

```json
{
  "proxy": "running",
  "time": "2026-04-13T15:00:00+08:00",
  "listen": "serverless",
  "checks": [
    {
      "name": "auth.docker.io",
      "url": "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull",
      "status": "HTTP 200",
      "latency": "66ms",
      "detail": "OK"
    }
  ]
}
```

### V2 探活

```bash
curl "https://你的域名/v2/"
```

期望：

- HTTP 200
- Header 含 `Docker-Distribution-Api-Version: registry/2.0`
- Body 为 `{}`

### 手动测试 Manifest 拉取

```bash
curl -s "https://你的域名/v2/library/alpine/manifests/latest" \
  -H "Accept: application/vnd.docker.distribution.manifest.v2+json" | head -20
```

## 支持的上游仓库

默认代理 Docker Hub。也可通过 `ns` 查询参数或域名前缀路由其他仓库：

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

## 注意事项

- Token 缓存为**实例内内存缓存**，属于 best-effort；冷启动或实例切换时可能失效
- Pages/Vercel 的网络出口、响应时长、文件大小限制以各平台实时规则为准
- 浏览器页面代理、Registry API 代理、CDN 重定向跟随均已保留

## 参考文档

- EdgeOne Pages Go Runtime：<https://pages.edgeone.ai/zh/document/go>
- EdgeOne Pages Go Functions 公告：<https://pages.edgeone.ai/zh/resources/pages-functions-support-python-and-go>
- Vercel Go Runtime：<https://vercel.com/docs/functions/runtimes/go>
- Vercel Rewrites：<https://vercel.com/docs/rewrites>
