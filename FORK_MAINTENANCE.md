# sub2api-plus 手册

私有 fork，相对官方版只加了一个功能：**自定义菜单的「打开方式」和 `plus:key_launch` 模块**。

## 目录

| 文档 | 什么时候看 |
|---|---|
| [docs-fork/key-launch.md](docs-fork/key-launch.md) | 要配一个带密钥跳转的菜单 |
| [docs-fork/local-build.md](docs-fork/local-build.md) | 本地构建、跑起来、构建报错 |
| [docs-fork/upstream-sync.md](docs-fork/upstream-sync.md) | 同步上游代码、解冲突、发版 |

> 放在 `docs-fork/` 而不是 `docs/`：上游 `.gitignore` 有 `docs/*` 规则，只白名单放行了几个上游文档，放进去不会进版本库。
