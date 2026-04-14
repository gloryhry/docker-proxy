# EdgeOne Pages 部署清单

当前仓库是 **EdgeOne 专用仓库**。

## 一、部署前检查

- [ ] 代码已位于 `edgeone-pages` 分支或独立 EdgeOne 仓库
- [ ] `cloud-functions/go.mod` 存在
- [ ] `cloud-functions/[[path]].go` 存在
- [ ] `[[path]].go` 同时包含完整代理逻辑与唯一入口函数 `Handler`
- [ ] 如本地安装了 EdgeOne CLI，可执行：

```bash
edgeone pages build
```

## 二、EdgeOne 控制台填写

导入 Git 仓库后，建议填写如下：

- **Framework Preset**：`Other`
- **Root Directory**：仓库根目录
- **Install Command**：留空
- **Build Command**：留空
- **Output Directory**：留空

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

如果构建日志再次出现：

```text
package xxx is not in std
```

说明仍依赖了额外内部包。

如果构建日志再次出现：

```text
redeclared in this block
```

说明仍然有多个路由文件重复声明共享逻辑。

当前推荐结构必须保持为：

- `cloud-functions/go.mod`
- `cloud-functions/[[path]].go`

即：**单文件入口 + 单文件逻辑**。

## 七、官方文档

- EdgeOne Pages Go Runtime：<https://pages.edgeone.ai/zh/document/go>
- EdgeOne Pages Build Guide：<https://pages.edgeone.ai/zh/document/build-guide>
- EdgeOne Pages Go Handler Template：<https://pages.edgeone.ai/templates/go-handler-template>
