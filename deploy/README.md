# Sub2API Deploy

本目录保留 Docker 部署、脚本安装和 systemd 部署相关文件。

主文档见：

```text
../README_CN.md
```

## Docker 部署

推荐使用本地目录持久化版本，方便备份和迁移：

```bash
cd deploy
cp .env.example .env
mkdir -p data postgres_data redis_data
docker compose -f docker-compose.local.yml up -d
```

默认镜像：

```text
ghcr.io/zluo003/sub2api:latest
```

查看日志：

```bash
docker compose -f docker-compose.local.yml logs -f sub2api
```

停止服务：

```bash
docker compose -f docker-compose.local.yml down
```

## 本地开发镜像

项目二次开发时建议在项目根目录构建本地镜像：

```bash
cd ..
docker build -t sub2api:dev .
cd deploy
docker compose -f docker-compose.dev.yml up -d
```

开发 compose 使用：

```text
sub2api:dev
```

不会从远程镜像仓库拉取应用镜像。

## 公共文件服务

管理后台的 `/admin/file-service` 是独立于 Yingzo 的公共模型基础设施，可供图片、视频、音频及后续模型服务共用。它负责：

- 在服务器本地、S3、MinIO 或 Cloudflare R2 之间选择存储后端。
- 配置独立公网基址、文件保留时长和 24 小时滚动上传配额。
- 检查本地目录或对象存储连接，并显示当前有效文件和占用空间。
- 将 Access Secret 加密保存到 PostgreSQL，保存后立即生效，无需重启服务。

推荐直接在管理后台保存配置，不需要在 `.env` 中填写文件服务参数。数据库中的 `file_storage_config` 始终优先；`FILE_SERVICE_*` 仅用于无人值守部署的启动回退，旧 `AGENT_ASSETS_*` 仅作升级兼容。

文件对模型公开时统一经过 sub2api 代理，不暴露对象键或 S3 预签名 URL：

```text
https://api-key.cc/media/{asset-uuid}/asset.png
https://api-key.cc/media/{asset-uuid}/asset.mp4
https://api-key.cc/media/{asset-uuid}/asset.mp3
```

这些 URL 没有查询参数，以真实媒体扩展名结尾，并支持 `GET`、`HEAD` 和 `Range`。对象过期后由服务端从本地或对象存储清理。

## Yingzo 私有发行

sub2api 是 Yingzo（影作）的公开产品入口、授权网关和私有安装包分发端。Yingzo 源码与安装包不得提交到 sub2api 的公开 Git 仓库。

管理员流程：

1. 打开 `/admin/yingzo`。
2. 将通信域名保存为 `https://api-key.cc`。该值保存在数据库设置中，不使用 `.env`。
3. 先创建 `schema 3` 草稿，再逐项上传固定七项产物：五个宿主包、macOS arm64 Runtime 和 Windows x64 Runtime。旧 `schema 2` 八项记录只保留历史读取与回滚。
4. 上传 CI 生成的单个 `yingzo-release-<version>.proof.json`（必须包含 `algorithm: Ed25519`、`key_id`、`manifest_base64` 和 `signature_base64`）；服务端重新计算每个文件的大小、SHA-256，并用 `YINGZO_RELEASE_PUBLIC_KEYS` 中的 Ed25519 公钥验证原始 manifest。
5. 发布到 `prerelease` 或 `stable` 通道。默认产品页只读取 stable；测试用户必须显式选择 prerelease。系统保证每个通道最多一个当前发布版本。
6. 验收通过后可将同一组已签名二进制提升为 stable，不重新构建；出现问题时可按通道回滚或停用。未发布的草稿和空白停用记录可永久删除并释放版本号，曾发布的记录只能停用，不能覆盖或永久删除。

安装包默认保存在持久化目录 `/app/data/releases/`（可用 `YINGZO_RELEASE_STORAGE_DIR` 改为其他绝对路径），PostgreSQL 只保存路径、大小、哈希和发行元数据，不保存二进制内容。公开产品页位于 `/yingzo`。登录用户选择 ChatGPT Work、Codex、Claude Cowork、Claude Desktop 或 Claude Code 后，可复制包含短期 bearer 下载地址、Runtime 检测深链和对应宿主官方安装命令的提示词；下载票据支持 `HEAD`、`Range` 和短期内重试。安装过程必须保留 `~/.yingzo/auth.json`。

发行证明的私钥只存在于受保护的 Yingzo release workflow；sub2api 只配置 base64 原始 32 字节公钥。密钥轮换前，旧公钥必须继续留在 `YINGZO_RELEASE_PUBLIC_KEYS`，直到所有使用它签名的发布版本不再发布或可回滚。

Yingzo 通过上述公共文件服务提交临时参考素材；其发行配置和安装包管理仍保留在 `/admin/yingzo`，不承载文件存储设置。

## 一键 Docker 准备脚本

可以在空目录中生成 `docker-compose.yml`、`.env` 和数据目录：

```bash
mkdir -p sub2api-deploy
cd sub2api-deploy
curl -sSL https://raw.githubusercontent.com/Zluo003/sub2api/main/deploy/docker-deploy.sh | bash
docker compose up -d
```

脚本会生成：

- `.env`
- `docker-compose.yml`
- `data/`
- `postgres_data/`
- `redis_data/`

请妥善保存 `.env` 中的 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY` 和数据库密码。

## 脚本安装

Linux systemd 安装脚本已保留：

```bash
curl -sSL https://raw.githubusercontent.com/Zluo003/sub2api/main/deploy/install.sh | sudo bash
```

安装位置：

```text
/opt/sub2api
/etc/sub2api
```

常用命令：

```bash
sudo systemctl status sub2api
sudo journalctl -u sub2api -f
sudo systemctl restart sub2api
```

脚本安装依赖 GitHub Releases 中的二进制包和 `checksums.txt`。如果本仓库还没有正式 release，安装脚本无法下载可用版本。

## 管理端一键更新

管理端更新功能会检查：

```text
https://github.com/Zluo003/sub2api/releases/latest
```

该功能只适合 release 二进制部署。Docker 部署请通过更新镜像和重启容器升级。

## 文件说明

| 文件 | 说明 |
| --- | --- |
| `.env.example` | Docker 环境变量模板 |
| `docker-compose.local.yml` | 推荐生产部署，使用本地目录持久化 |
| `docker-compose.yml` | 命名卷版本 |
| `docker-compose.dev.yml` | 本地二开镜像版本 |
| `docker-compose.standalone.yml` | 仅应用容器，外接 PostgreSQL/Redis |
| `docker-deploy.sh` | 空目录一键生成 Docker 部署文件 |
| `install.sh` | Linux systemd 二进制安装/升级脚本 |
| `sub2api.service` | systemd service 模板 |
| `config.example.yaml` | 应用配置示例 |

## 数据迁移

使用 `docker-compose.local.yml` 时，完整部署数据都在当前目录：

```text
.env
data/
postgres_data/
redis_data/
```

迁移到新机器时，先停服务，再整体打包复制：

```bash
docker compose -f docker-compose.local.yml down
tar czf sub2api-deploy.tar.gz .env data postgres_data redis_data docker-compose.local.yml
```

恢复后重新启动：

```bash
docker compose -f docker-compose.local.yml up -d
```

## 数据库空间诊断与安全清理

`tools/cleanup_nonessential_database.sh` 用于检查并清理可重建的数据库数据。它默认只报告，不修改数据库：

```bash
./tools/cleanup_nonessential_database.sh
```

默认连接 `sub2api-postgres` 容器中的 `sub2api` 数据库。其他容器可显式指定：

```bash
./tools/cleanup_nonessential_database.sh \
  --container sub2api-postgres-dev \
  --database sub2api
```

也可以使用本机 `psql` 直连 PostgreSQL；认证沿用 `.pgpass` 或 `psql` 密码提示，不在命令行传密码：

```bash
./tools/cleanup_nonessential_database.sh \
  --host 127.0.0.1 \
  --port 5432 \
  --user sub2api \
  --database sub2api
```

先审阅报告并完成数据库备份，再执行清理。脚本会要求输入数据库名确认：

```bash
./tools/cleanup_nonessential_database.sh --apply
```

默认清理内容只有：

- 将历史 `video_tasks.request_json` 和 `upstream_response_json` 重置为空对象，不删除视频任务、状态、结果地址、扣费或退款记录。
- 删除已过期的生成报价和幂等记录。
- 删除过期超过 7 天的设备授权记录。
- 只删除 `deleted_at` 已存在且超过 7 天的临时资产元数据，不删除仍有效的文件记录。

运维日志默认不删除。需要时必须显式指定保留天数：

```bash
./tools/cleanup_nonessential_database.sh \
  --apply \
  --purge-ops-logs \
  --log-retention-days 30
```

普通清理后的 `VACUUM (ANALYZE)` 会让 PostgreSQL 复用空间，但不会立即缩小宿主机上的数据文件。确认业务低峰、磁盘有足够临时空间，并允许短时独占锁后，才执行：

```bash
./tools/cleanup_nonessential_database.sh --apply --vacuum-full
```

`VACUUM FULL` 只压缩 `video_tasks`，不会扩大到消费记录或其他核心业务表。脚本不会删除 `usage_logs`、用户、API Key、分组、账号、模型定价、视频任务行或 Yingzo 发行记录。
