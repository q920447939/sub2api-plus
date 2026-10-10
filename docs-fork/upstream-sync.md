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

