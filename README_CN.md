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

完整的媒体上传、`attachment://N` 顺序映射、JSON/multipart 请求、Node.js/Python/curl 示例、错误码和部署要求请参阅：[Seedance 视频与媒体输入接口](docs/SEEDANCE_VIDEO_MEDIA_API.md)。

### 一次提交本地图片、视频和音频

`/v1/videos` 同时接受普通 JSON 和 multipart 请求。已经是公网 `http(s)` URL 的媒体会原样保留并直接交给上游，不会被 sub2api 下载或重新上传。

上述规则只针对下游提交的输入素材。视频任务完成后，上游返回的结果 URL 会由 sub2api 在服务端流式下载并转存到共享 File Service；任务结果和 usage log 只返回 `https://sub2api.example.com/media/{id}/asset.mp4`（或 `.mov`），不会暴露供应商原始 URL。转存完成前任务保持 `processing`，转存失败会在生命周期轮询期限内重试，不会回退为供应商 URL。

仅当后台视频账号配置为 `video_provider = jingyu` 时，上游任务状态改为完成回调：创建请求自动携带每任务 `callback_url`/`callback_secret`，Sub2API 通过 `POST /api/v1/webhooks/jingyu/videos/{id}` 验证 `X-NewAPI-Signature` 并处理成功或失败终态，不再向 Jingyu 发送状态 GET 轮询。Aigod 视频轮询和所有图片接口保持原有行为。详细签名、超时退款和部署放行要求见视频接口文档第 6.2 节。

如果下游需要在一次请求中上传本地文件，使用 multipart：

- JSON 请求放在 `request` 字段，也支持字段名 `json` 或 `body`。
- 本地文件使用重复的 `file` 字段。
- 文件顺序从 `0` 开始；请求体中的媒体 URL 使用 `attachment://0`、`attachment://1` 等序号引用对应文件。
- sub2api 按文件顺序上传，替换对应 URL 后，再把改写后的 JSON 提交到 Seedance 上游。
- 未使用 `attachment://N` 的公网 URL 保持原值，整个过程不会触发下载。

示例：

```bash
curl -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <api-key>' \
  -F 'request={"model":"seedance-2.0","prompt":"保持人物和动作连续","ability_code":"video_reference_to_video","resolution":"720p","duration":5,"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"attachment://0"}},{"type":"video_url","role":"reference_video","duration_seconds":5,"video_url":{"url":"attachment://1"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://cdn.example.com/music.mp3"}}]};type=application/json' \
  -F 'file=@./reference.png;type=image/png' \
  -F 'file=@./reference.mp4;type=video/mp4'
```

multipart 请求中的本地素材和视频生成结果 URL 都使用共享文件服务配置的公网根地址。推荐在后台 `File Service` 中配置 `Public Base URL`，例如 `https://sub2api.example.com`；也可以使用 `FILE_SERVICE_PUBLIC_BASE_URL` 作为部署启动回退配置。临时素材和转存结果沿用 File Service 的留存期、每日数量及字节配额。多实例或 API/worker 分离部署使用 local 后端时必须共享 `agent-assets` 目录，否则应配置 S3/R2 等共享存储。

本地素材只接受随 `POST /v1/videos` 提交的一次性 multipart 请求。Sub2API 不提供独立素材上传或素材元数据查询接口，避免把文件服务当作通用公网存储使用。

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
