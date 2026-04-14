# EdgeOne Pages 部署清单

当前仓库是 **EdgeOne 专用仓库**。

## 一、部署前检查

- [ ] 代码已位于 `edgeone-pages` 分支或独立 EdgeOne 仓库
- [ ] `cloud-functions/go.mod` 存在
- [ ] `cloud-functions/index.go` 存在
- [ ] `cloud-functions/internal/proxy/handler.go` 存在
- [ ] 本地验证通过：

```bash
cd "cloud-functions"
go test ./...
go build ./...
```

## 二、EdgeOne 控制台填写

导入 Git 仓库后，建议填写如下：

- **Framework Preset**：`Other`
- **Root Directory**：仓库根目录
- **Install Command**：留空
- **Build Command**：留空
- **Output Directory**：留空

说明：

- 当前仓库不是静态站点，无需输出目录
- 当前仓库不是前端框架项目，无需构建命令
- Go 运行时入口为 `cloud-functions/index.go`

## 三、部署步骤

1. [ ] 打开 EdgeOne Pages 控制台
2. [ ] 导入当前分支/仓库
3. [ ] Root Directory 保持为仓库根目录
4. [ ] Framework Preset 选 `Other`
5. [ ] Install / Build / Output 全部留空
6. [ ] 等待部署完成
7. [ ] 记录分配域名

## 四、部署后立即验证

```bash
curl -i "https://你的域名/v2/"
curl "https://你的域名/health"
curl -I "https://你的域名/"
```

期望：

- [ ] `/v2/` 返回 `200`
- [ ] Header 包含 `Docker-Distribution-Api-Version: registry/2.0`
- [ ] `/health` 返回 JSON
- [ ] `/` 可访问

## 五、Docker 验收

```bash
docker pull 你的域名/library/alpine:latest
docker pull 你的域名/library/nginx:latest
```

## 六、排障要点

如果部署后仍返回 EdgeOne 默认 404，优先检查：

- [ ] 仓库中是否仍混入其他平台入口
- [ ] `cloud-functions/` 目录是否存在且在仓库根目录下
- [ ] 是否误填了 Build Command / Output Directory
- [ ] 是否实际部署的是正确分支
- [ ] 自定义域名是否已正确绑定到当前项目

## 七、官方文档

- EdgeOne Pages Go Runtime：<https://pages.edgeone.ai/zh/document/go>
- EdgeOne Pages Build Guide：<https://pages.edgeone.ai/zh/document/build-guide>
- EdgeOne Pages Go Handler Template：<https://pages.edgeone.ai/templates/go-handler-template>
