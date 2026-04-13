# Docker Proxy 部署清单

适用于当前仓库的两个目标平台：

- **Vercel**
- **腾讯 EdgeOne Pages**

当前仓库结构基于以下事实编写：

- 根目录存在 `go.mod`
- Vercel 入口为 `api/index.go`
- Vercel 路由配置为 `vercel.json`
- EdgeOne 入口为 `cloud-functions/index.go`
- EdgeOne 子模块为 `cloud-functions/go.mod`

---

## 一、部署前通用检查

### 代码侧检查

- [ ] 根目录存在 `go.mod`
- [ ] Vercel 入口存在：`api/index.go`
- [ ] Vercel 路由配置存在：`vercel.json`
- [ ] EdgeOne 入口存在：`cloud-functions/index.go`
- [ ] EdgeOne 子模块存在：`cloud-functions/go.mod`
- [ ] 本地验证通过：

```bash
go test ./...
go build ./...
cd "cloud-functions" && go test ./... && go build ./...
```

### Git 准备

- [ ] 代码已推送到 GitHub / GitLab / Bitbucket
- [ ] 默认生产分支已确定（通常是 `main`）

---

## 二、Vercel 部署清单

### A. 控制台部署

1. [ ] 打开 Vercel 控制台：<https://vercel.com/new>
2. [ ] 选择 **Import Git Repository**
3. [ ] 授权 Git 提供商并选择当前仓库
4. [ ] 在项目配置页确认：
   - [ ] **Root Directory**：仓库根目录
   - [ ] `go.mod` 位于项目根目录
   - [ ] 不需要自定义 Build Command
   - [ ] 不需要 Output Directory
5. [ ] 点击 **Deploy**
6. [ ] 等待首次部署完成
7. [ ] 打开分配的 `*.vercel.app` 域名验证

### B. 部署后立即验证

```bash
curl -i "https://你的项目.vercel.app/v2/"
curl "https://你的项目.vercel.app/health"
curl -I "https://你的项目.vercel.app/"
```

期望：

- [ ] `/v2/` 返回 `200`
- [ ] 响应头包含 `Docker-Distribution-Api-Version: registry/2.0`
- [ ] `/health` 返回 JSON
- [ ] `/` 可访问

### C. 自定义域名

1. [ ] 进入项目 → **Settings** → **Domains**
2. [ ] 添加你的域名
3. [ ] 按 Vercel 提示配置 DNS
   - [ ] 子域名通常配置 **CNAME**
   - [ ] 根域名按控制台提示配置
4. [ ] 等待域名验证成功
5. [ ] 再次验证：

```bash
curl -i "https://你的自定义域名/v2/"
curl "https://你的自定义域名/health"
```

### D. Docker 客户端验收

```bash
docker pull 你的自定义域名/library/alpine:latest
docker pull 你的自定义域名/library/nginx:latest
```

如果要作为 mirror 使用：

- [ ] 将域名写入 Docker 的 `registry-mirrors`

---

## 三、EdgeOne Pages 部署清单

> 当前实现采用 **Go Runtime + `cloud-functions/index.go` 单入口**。  
> 这是基于 EdgeOne 官方 Go 文档中支持标准库 `net/http` 的模式整理出的落地方案。

### A. 控制台部署

1. [ ] 打开 EdgeOne Pages 控制台
2. [ ] 创建项目 / 导入 Git 仓库
3. [ ] 选择当前仓库
4. [ ] 在构建配置页检查：
   - [ ] **Root Directory**：仓库根目录
   - [ ] 平台能识别 `cloud-functions/index.go`
   - [ ] `cloud-functions/go.mod` 已在仓库中
   - [ ] 若平台提示框架/构建设置，优先使用自动识别
5. [ ] 选择 **加速区域**
   - [ ] 若只面向海外/免备案，优先选非中国大陆区域
   - [ ] 若要绑定中国大陆可访问域名，注意备案要求
6. [ ] 点击开始部署
7. [ ] 等待部署成功，记录默认分配域名

### B. 部署后立即验证

```bash
curl -i "https://你的项目.edgeone.app/v2/"
curl "https://你的项目.edgeone.app/health"
curl -I "https://你的项目.edgeone.app/"
```

期望：

- [ ] `/v2/` 返回 `200`
- [ ] `/health` 正常
- [ ] `/` 可访问

### C. 自定义域名

1. [ ] 进入项目详情 → **域名管理**
2. [ ] 点击 **添加自定义域名**
3. [ ] 输入根域名或子域名
4. [ ] 按弹窗提示完成域名归属权验证
5. [ ] 按平台提供的记录值配置 DNS
   - [ ] 一般按提示添加 **CNAME**
6. [ ] 等待 Pages 检测到 DNS 生效
7. [ ] 验证：

```bash
curl -i "https://你的自定义域名/v2/"
curl "https://你的自定义域名/health"
```

### D. EdgeOne 特别注意

- [ ] 如果加速区域选了 **中国大陆可用区** 或 **全球可用区（含中国大陆）**，自定义域名需先备案
- [ ] 若域名托管在 Cloudflare，根域名接入可能受限制，优先使用子域名
- [ ] 每次推送到生产分支后，确认自动触发新部署

### E. Docker 客户端验收

```bash
docker pull 你的自定义域名/library/alpine:latest
docker pull 你的自定义域名/library/nginx:latest
```

---

## 四、上线后的最小验收清单

### HTTP 验收

- [ ] `GET /` 返回首页
- [ ] `GET /search?q=nginx` 正常
- [ ] `GET /v2/` 正常
- [ ] `GET /health` 正常
- [ ] `GET /token?...` 正常

### Registry 验收

- [ ] `docker pull nginx`
- [ ] `docker pull alpine`
- [ ] `docker pull 你的域名/library/nginx:latest`
- [ ] blob 下载不报 `401` / `302` 死循环

### 多上游验收

- [ ] `?ns=ghcr.io`
- [ ] `?ns=registry.k8s.io`

---

## 五、建议的实际执行顺序

### 推荐顺序：先 Vercel，后 EdgeOne

1. [ ] 先部署到 Vercel
2. [ ] 验证 `/v2/`、`/health`、`docker pull`
3. [ ] 再部署 EdgeOne
4. [ ] 最后绑定正式域名

原因：

- Vercel 对当前 `/api/*.go + root go.mod + rewrite` 结构更直接
- 先用 Vercel 验证共享核心逻辑，能更快定位问题
- EdgeOne 再做平台适配验证，排障边界更清晰

---

## 六、官方文档

### Vercel

- 导入项目：<https://vercel.com/docs/getting-started-with-vercel/import>
- Git 部署：<https://vercel.com/docs/deployments/git>
- Go Runtime：<https://vercel.com/docs/functions/runtimes/go>
- Rewrites：<https://vercel.com/docs/rewrites>
- 自定义域名：<https://vercel.com/docs/domains/working-with-domains/add-a-domain>

### EdgeOne Pages

- 导入 Git 仓库：<https://pages.edgeone.ai/zh/document/importing-a-git-repository>
- Go Runtime：<https://pages.edgeone.ai/zh/document/go>
- 构建设置：<https://pages.edgeone.ai/zh/document/build-guide>
- 域名管理概览：<https://pages.edgeone.ai/zh/document/domain-overview>
- 添加自定义域名：<https://pages.edgeone.ai/zh/document/custom-domain>
- 配置 CNAME：<https://pages.edgeone.ai/zh/document/how-to-configure-a-dns-cname-record>
