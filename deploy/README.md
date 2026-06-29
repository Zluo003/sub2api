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
