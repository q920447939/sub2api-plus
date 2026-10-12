# 同步上游

日常运维用不到，只在合并官方代码时看。

```
upstream  →  https://github.com/Wei-Shaw/sub2api        （官方）
origin    →  https://github.com/q920447939/sub2api-plus  （私有 fork）
```

分支：

- `main` — 跟踪 `upstream/main`，不直接开发
- `plus/menu-enhancements` — 二开分支，基于 tag `v0.2.8`

## 步骤

```bash
git fetch upstream

git checkout main
git merge upstream/main

git checkout plus/menu-enhancements
git rebase main
```

rebase 完之后必须强推（rebase 重写了哈希，普通 push 会被拒）：

```bash
git push --force-with-lease origin plus/menu-enhancements
```

别用 VSCode 的「同步」按钮，它走 merge，会把旧提交再合进来。

## 冲突处理

| 文件 | 怎么办 |
|---|---|
| `backend/cmd/server/wire_gen.go` | 取上游版本，然后在 `backend/` 跑 `go run github.com/google/wire/cmd/wire ./cmd/server` 重新生成 |
| 其他上游文件里带 `// [plus]` 的行 | 保留标记行，其余取上游 |
| `backend/internal/plus/`、`frontend/src/plus/` | 全取本分支 |

快速找到所有接入点：

```bash
git grep '\[plus\]'
```

## 发版

推一个 `plus/v*` 格式的 tag，GitHub Actions 自动构建镜像推到 GHCR。

版本号格式：`上游版本-plus.序号`，例如 `0.2.15-plus.1`。

```bash
# 确认工作区干净
git status

# 打 tag 并推送（触发 CI）
git tag plus/v0.2.15-plus.1
git push origin plus/v0.2.15-plus.1
```

推完后去 GitHub → Actions → Plus Release 看进度，约 5-10 分钟。

构建完的镜像：
```
ghcr.io/q920447939/sub2api-plus:0.2.15-plus.1
ghcr.io/q920447939/sub2api-plus:latest
```

**第一次用前**，把镜像包设为公开：GitHub → Settings → Packages → sub2api-plus → Change visibility → Public

**撤销误打的 tag：**
```bash
git tag -d plus/v0.2.15-plus.1                    # 删本地
git push origin --delete plus/v0.2.15-plus.1      # 删远端（不会触发 CI）
```

## 二开改了哪些上游文件

只在解冲突时对照。

**后端**

| 文件 | 改动 |
|---|---|
| `internal/handler/dto/settings.go` | `CustomMenuItem` 加 `OpenMode`、`PassParams`、`PlusConfig`；`ParseUserVisibleMenuItems` 清空 `PlusConfig` |
| `internal/handler/admin/setting_handler_update.go` | 菜单 URL 校验加 `plus:` 分支 |
| `internal/server/router.go` | import `plus`，`registerRoutes` 末尾加 `plus.RegisterRoutes(...)` |
| `internal/service/setting_public.go` | `filterUserVisibleMenuItems` 剥离 `plus_config`；`parseCustomMenuItemURLs` 支持 `plus:` |

**前端**

| 文件 | 改动 |
|---|---|
| `types/index.ts` | `CustomMenuItem` 加三个可选字段；新增 `KeyLaunchConfig` |
| `components/layout/AppSidebar.vue` | 新增 `navItemForCustomMenu()` |
| `utils/embedded-url.ts` | `buildEmbeddedUrl` 加第 6 个参数 `passParams` |
| `views/admin/SettingsView.vue` | 插入 `<PlusMenuSettings>` |
| `views/user/CustomPageView.vue` | `plus:` URL 重定向到 `/launch/:id` |
| `router/index.ts` | 新增 `/launch/:id` |
| `i18n/locales/{zh,en}/index.ts` | 引入 `plus` 语言包 |
| `i18n/locales/{zh,en}/admin/settings.ts` | 新增 `plusMenu.*` 键 |

**fork 自己的文件（不会冲突）**

```
backend/internal/plus/           key_launch.go / plus_handler.go / plus_routes.go
backend/internal/service/plus_bridge.go
frontend/src/plus/               api.ts / LaunchView.vue / PlusMenuSettings.vue
frontend/src/i18n/locales/{zh,en}/plus.ts
deploy/                          Dockerfile.plus-local / docker-compose.plus-local.yml / .env.plus-local
.github/workflows/plus-release.yml
```

## 新增 plus 模块

1. `backend/internal/plus/` 新建 `xxx_launch.go`，定义 `XxxConfig` 和业务逻辑
2. `plus_handler.go` 加 handler 方法
3. `plus_routes.go` 加一行路由
4. `PlusMenuSettings.vue` 加 `plus:xxx` 的配置分支
5. 需要新页面就在 `frontend/src/plus/` 建组件，在 `router/index.ts` 注册路由

不用改上游文件。
