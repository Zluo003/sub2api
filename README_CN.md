# Sub2API

Sub2API 是一个自用二次开发版 AI API 网关，基于原 Sub2API 项目继续维护。当前重点是多平台账号管理、用户/API Key 管理、计费、用量日志，以及新增的 Seedance 2.0 视频网关能力。

仓库地址：

```text
https://github.com/Zluo003/sub2api
```

## 当前改动重点

- 新增 `video` 平台。
- 新增 OpenAI 风格异步视频接口：
  - `POST /v1/videos`
  - `GET /v1/videos/{id}`
- 接入 Seedance 2.0 / Seedance 2.0 fast 视频模型。
- 视频上游通过 Aigod API Key 账号接入。
- 管理端支持视频平台账号、视频分组和按“模型 + 分辨率 + 秒数”的计费规则。
- 视频任务接入现有用户、API Key、分组、余额、订阅额度和用量记录。
- 视频成功扣费，失败记录退费；用量记录展示端点和耗时信息。

## 主要功能

- 多平台上游账号管理。
- 用户、API Key、分组、订阅和余额管理。
- 请求调度、并发控制、限流和健康状态管理。
- Token / 图片 / 视频用量记录与计费。
- 管理后台在线配置、查看日志和执行系统更新。
- Docker 部署、本地目录持久化、PostgreSQL 和 Redis 支持。

## Docker 本地二开部署

当前项目建议使用本地镜像进行二次开发。

```bash
cd /Users/zluo/Dev/sub2api
docker build -t sub2api:dev .
cd deploy
docker compose -f docker-compose.dev.yml up -d
```

查看服务：

```bash
docker ps
docker logs -f sub2api-dev
```

默认访问地址：

```text
http://127.0.0.1:8080
```

开发环境使用的数据目录在：

```text
deploy/data/
deploy/postgres_data/
deploy/redis_data/
```

这些目录是本地运行数据，不应提交到 Git。

## Docker 正式部署

仓库保留原项目的 Docker 部署能力，并已将默认镜像源调整为：

```text
ghcr.io/zluo003/sub2api:latest
```

首次准备部署目录：

```bash
mkdir -p sub2api-deploy
cd sub2api-deploy
curl -sSL https://raw.githubusercontent.com/Zluo003/sub2api/main/deploy/docker-deploy.sh | bash
docker compose up -d
```

也可以直接使用仓库内的部署文件：

```bash
cd deploy
cp .env.example .env
mkdir -p data postgres_data redis_data
docker compose -f docker-compose.local.yml up -d
```

重要配置在 `deploy/.env` 中，包括：

- `POSTGRES_PASSWORD`
- `JWT_SECRET`
- `TOTP_ENCRYPTION_KEY`
- `ADMIN_EMAIL`
- `ADMIN_PASSWORD`

`JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY` 必须长期固定。恢复旧数据时如果密钥不一致，历史加密字段可能无法解密。

## 脚本安装

原项目的一键安装脚本功能已保留，并已改为从本仓库读取 GitHub Releases：

```bash
curl -sSL https://raw.githubusercontent.com/Zluo003/sub2api/main/deploy/install.sh | sudo bash
```

脚本安装适合 Linux 服务器，会安装到：

```text
/opt/sub2api
/etc/sub2api
```

并创建 systemd 服务：

```bash
sudo systemctl status sub2api
sudo journalctl -u sub2api -f
sudo systemctl restart sub2api
```

注意：脚本安装依赖本仓库 GitHub Releases 中的二进制构建产物。只有创建正式 release 并上传 `sub2api_<version>_<os>_<arch>.tar.gz` 和 `checksums.txt` 后，脚本安装/升级才可用。

## 管理端一键更新

管理端的一键更新功能已保留。当前后台更新检查默认读取：

```text
https://github.com/Zluo003/sub2api/releases/latest
```

使用限制：

- 仅适合 release 二进制部署。
- 源码运行、Docker 本地开发镜像、`go run` 不适合作为后台一键更新目标。
- 更新依赖 GitHub Release asset 名称与 GoReleaser 输出一致。
- 更新完成后需要重启服务。

发布新版本后，管理后台可检测新版本并下载替换当前二进制。

## 发布 Release

仓库保留 `.github/workflows/release.yml` 和 GoReleaser 配置。

发布正式版本：

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions 会构建：

- 多平台二进制包。
- `checksums.txt`。
- GHCR Docker 镜像。

镜像地址格式：

```text
ghcr.io/zluo003/sub2api:<version>
ghcr.io/zluo003/sub2api:latest
```

如果只发布简化版镜像，请注意简化 release 不上传二进制包，因此脚本安装和后台一键更新无法使用。

## 本地开发

后端：

```bash
cd backend
go test ./...
go run ./cmd/server
```

前端：

```bash
cd frontend
pnpm install
pnpm dev
pnpm typecheck
pnpm test:run
```

常用验证：

```bash
cd backend && go test ./...
cd ../frontend && pnpm typecheck
cd .. && git diff --check
```

## 数据备份与恢复

项目根目录的生产备份文件不应提交到 Git。

恢复测试建议使用临时 PostgreSQL 容器，不要直接覆盖当前开发库：

#### ⚠️ 重要：创建管理员账号

初始管理员账号**只能通过 setup 向导创建**（首次启动时访问 `http://<host>:8080`）。`config.yaml` 中的 `default.admin_email` / `default.admin_password` 字段**不会被用来创建管理员**——它们只是出于历史原因保留在模板里。

由于上面第 5 步预先创建了 `config.yaml`，**setup 向导在首次启动时会被跳过**：服务检测到 config 已存在，会直接进入正常模式，此时 `users` 表为空，首次登录会返回 `invalid email or password`。

**创建管理员的两种方式：**

1. **推荐——让向导自动生成 `config.yaml`：** 跳过上面的第 5 步（不要执行 `cp`）。直接运行 `./sub2api`，访问 `http://localhost:8080`，向导会引导你完成数据库、Redis 和管理员账号配置，并自动写出 `config.yaml`。

2. **如果你已经创建了 `config.yaml`：** 首次启动前先把它临时移走以触发向导，完成后再恢复：
   ```bash
   mv config.yaml config.yaml.bak
   ./sub2api        # 向导在 http://localhost:8080 启动，并生成新的 config.yaml
   # 向导完成后 Ctrl+C 停服，再恢复你的配置：
   mv config.yaml.bak config.yaml
   ./sub2api        # 重启进入正常模式，用刚创建的管理员登录
   ```

```bash
docker run --rm -d --name sub2api-restore-check-pg \
  -e POSTGRES_PASSWORD=restore_check \
  -e POSTGRES_DB=sub2api_restore_check \
  -p 127.0.0.1:55432:5432 postgres:18-alpine

gunzip -c backups_*.sql.gz | docker exec -i sub2api-restore-check-pg \
  psql -U postgres -d sub2api_restore_check
```

恢复后再用当前代码启动或执行迁移，确认旧数据兼容新 schema。

## 视频接口

创建视频任务：

```http
POST /v1/videos
Authorization: Bearer <api-key>
Content-Type: application/json
```

示例：

```json
{
  "model": "seedance-2.0",
  "prompt": "a cinematic shot",
  "content": [
    {
      "type": "text",
      "text": "a cinematic shot"
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true
}
```

查询任务：

```http
GET /v1/videos/{id}
Authorization: Bearer <api-key>
```

下游响应不会暴露上游供应商名称、上游任务 ID、上游原始错误或上游响应结构。

## 注意事项

- 本项目为二次开发版本，请以本仓库代码和部署文件为准。
- `deploy/.env`、`deploy/data/`、`deploy/postgres_data/`、`deploy/redis_data/` 和根目录数据库备份都不应提交。
- 生产环境必须固定 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY` 和数据库密码。
- 使用第三方上游服务时，请自行确认合规风险、服务条款和账号安全。
