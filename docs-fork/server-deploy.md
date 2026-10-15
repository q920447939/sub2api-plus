# 服务器部署 / 更新

服务器地址：`server97`（测试）、`sub2Prod`（正式）

代码路径：`/home/sub2api_new/sub2api/deploy`

---

## 首次切换（官方镜像 → plus 镜像）

> 按顺序操作：先 server97 确认正常，再操作 sub2Prod。

**第一步：本地先打好 tag，触发 CI 构建镜像**

```bash
# 在本地 plus/menu-enhancements 分支执行
git tag plus/v0.2.16-plus.1
git push origin plus/v0.2.16-plus.1
```

去 GitHub → Actions → Plus Release 确认绿灯，约 5-10 分钟。
构建完的镜像：`ghcr.io/q920447939/sub2api-plus:0.2.16-plus.1`

**第二步：把新的 compose 文件传到服务器**

在本地执行：

```bash
scp deploy/docker-compose.plus.yml server97:/home/sub2api_new/sub2api/deploy/
scp deploy/docker-compose.plus.yml sub2Prod:/home/sub2api_new/sub2api/deploy/
```

**第三步：在 server97 切换（先测试）**

```bash
ssh server97
cd /home/sub2api_new/sub2api/deploy

# 停旧容器（数据文件不会丢，只停容器）
docker compose -f docker-compose.local.yml down

# 拉新镜像并启动（GHCR 是公开的，不需要登录）
docker compose -f docker-compose.plus.yml pull
docker compose -f docker-compose.plus.yml up -d

# 确认启动成功
docker compose -f docker-compose.plus.yml ps
docker compose -f docker-compose.plus.yml logs --tail=50 sub2api
```

健康检查：打开 `http://server97:8080`，登录，确认菜单功能可用。

**第四步：确认 server97 正常后，操作 sub2Prod**

```bash
ssh sub2Prod
cd /home/sub2api_new/sub2api/deploy

docker compose -f docker-compose.local.yml down
docker compose -f docker-compose.plus.yml pull
docker compose -f docker-compose.plus.yml up -d

docker compose -f docker-compose.plus.yml ps
docker compose -f docker-compose.plus.yml logs --tail=50 sub2api
```

---

## 后续版本更新

CI 构建出新镜像后（推了新 tag），在两台服务器上依次执行：

```bash
cd /home/sub2api_new/sub2api/deploy
docker compose -f docker-compose.plus.yml pull
docker compose -f docker-compose.plus.yml up -d
```

顺序：先 server97，确认没问题，再 sub2Prod。

---

## 回滚

切回官方镜像：

```bash
cd /home/sub2api_new/sub2api/deploy
docker compose -f docker-compose.plus.yml down
docker compose -f docker-compose.local.yml up -d
```

数据不会丢失（postgres_data / redis_data / data 目录没动）。

---

## 回滚到指定版本

如果 latest 有问题，想用上一个稳定版本：

```bash
# 修改 docker-compose.plus.yml 里的 image 行，把 latest 换成具体版本号
# 例：ghcr.io/q920447939/sub2api-plus:0.2.15-plus.1
docker compose -f docker-compose.plus.yml up -d
```
