# Docker Image

Sub2API 默认发布到 GitHub Container Registry：

```text
ghcr.io/zluo003/sub2api:latest
```

拉取镜像：

```bash
docker pull ghcr.io/zluo003/sub2api:latest
```

最小运行示例：

```yaml
services:
  sub2api:
    image: ghcr.io/zluo003/sub2api:latest
    ports:
      - "8080:8080"
    environment:
      - AUTO_SETUP=true
      - DATABASE_HOST=postgres
      - DATABASE_PORT=5432
      - DATABASE_USER=sub2api
      - DATABASE_PASSWORD=change_this_secure_password
      - DATABASE_DBNAME=sub2api
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - JWT_SECRET=change_this_fixed_secret
      - TOTP_ENCRYPTION_KEY=change_this_fixed_secret
    volumes:
      - ./data:/app/data
```

推荐直接使用本目录的 compose 文件：

```bash
cp .env.example .env
mkdir -p data postgres_data redis_data
docker compose -f docker-compose.local.yml up -d
```

二次开发使用本地镜像：

```bash
cd ..
docker build -t sub2api:dev .
cd deploy
docker compose -f docker-compose.dev.yml up -d
```

镜像 release 由 `.github/workflows/release.yml` 和 GoReleaser 生成。创建 tag 后会推送：

```text
ghcr.io/zluo003/sub2api:<version>
ghcr.io/zluo003/sub2api:latest
```
