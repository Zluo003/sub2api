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

## Yingzo 私有发行

sub2api 是 Yingzo（影作）的公开产品入口、授权网关和私有安装包分发端。Yingzo 源码与安装包不得提交到 sub2api 的公开 Git 仓库。

管理员流程：

1. 打开 `/admin/yingzo`。
2. 将通信域名保存为 `https://api-key.cc`。该值保存在数据库设置中，不使用 `.env`。
3. 上传 Yingzo 生成的 `.tar.gz`；可同时填写 CI 生成的 SHA-256 和签名。
4. 服务端重新计算 SHA-256；只有校验成功的包会进入草稿列表。
5. 发布草稿版本。系统保证同一时间只有一个当前发布版本。
6. 出现问题时可回滚到历史版本，或停用当前版本。

安装包默认保存在 sub2api 的私有数据目录 `agent-assets/releases/`，不会进入 Git。公开产品页位于 `/yingzo`。登录用户选择 Codex 或 Claude Code 后，可复制包含十分钟有效一次性下载地址、版本、SHA-256 和对应宿主安装命令的提示词。安装过程必须保留 `~/.yingzo/auth.json`。

临时参考素材可使用本地文件、MinIO、R2 或其他 S3 兼容对象存储作为内部后端，但绝不向模型暴露对象键或预签名 URL。模型收到的地址统一为：

```text
https://api-key.cc/media/{asset-uuid}/asset.png
https://api-key.cc/media/{asset-uuid}/asset.mp4
https://api-key.cc/media/{asset-uuid}/asset.mp3
```

这些 URL 没有查询参数，以真实媒体扩展名结尾。UUIDv4 提供不可猜测性，sub2api 负责代理 `GET`、`HEAD` 和 `Range` 请求；旧的 `/temporary-assets/:token` 路由仅用于向后兼容。对象在 24 小时过期后由服务端从本地或 S3 兼容存储删除。

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
