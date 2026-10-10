# 本地构建测试

**从仓库根目录运行**，不要在 `deploy/` 里。

```bash
# 构建并启动（首次约 5–10 分钟）
sudo docker compose \
  -f deploy/docker-compose.plus-local.yml \
  --env-file deploy/.env.plus-local \
  up --build -d

# 只重建应用，不动数据库
sudo docker compose \
  -f deploy/docker-compose.plus-local.yml \
  --env-file deploy/.env.plus-local \
  up --build -d sub2api-plus

# 日志
sudo docker compose -f deploy/docker-compose.plus-local.yml logs -f sub2api-plus

# 停止（保留数据）
sudo docker compose -f deploy/docker-compose.plus-local.yml down

# 停止并清数据（重新初始化）
sudo docker compose -f deploy/docker-compose.plus-local.yml down -v
```

访问 `http://localhost:8090`，账号密码在 `deploy/.env.plus-local`。

`--env-file` 不能省，缺了就报 `required variable POSTGRES_PASSWORD is missing a value`，连 `ps` 和 `logs` 都用不了。
