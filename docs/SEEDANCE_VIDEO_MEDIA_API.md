# Seedance 视频与媒体输入接口

本文档描述 Sub2API 当前的视频媒体输入协议，适用于需要向 Seedance 上游提交图片、视频和音频参考素材的下游客户端。

当前接口支持两种提交模式：

| 模式 | 适用场景 | sub2api 对媒体的处理 |
| --- | --- | --- |
| JSON + 公网 URL | 素材已经在 CDN、对象存储或其他公网服务上 | 保留原始 URL，直接提交上游，不下载、不重新上传 |
| multipart + `attachment://N` | 下游只有本地图片、视频或音频 | 按 multipart 文件顺序上传，替换对应 URL，再提交上游 |
| 上游生成结果 URL | Seedance 任务已经生成完成 | Sub2API 强制下载并转存，只向下游返回 Sub2API `/media/*` URL |

下游请求协议不随上游账号变化。选择 YCYAPI 视频账号时，Sub2API 会在上游适配层读取规范化请求中的图片、视频和音频 URL，并转换为 YCYAPI 要求的真实 `multipart/form-data` 文件字段；下游仍按本文的 JSON URL 或 `attachment://N` 协议提交。选择 newtoken 视频账号时，Sub2API 在上游适配层沿用 JSON URL 协议，把素材 URL 拆分到 newtoken 的 `first_frame`、`last_frame`、`extra_images`、`extra_videos`、`extra_audios` 字段，同样不改变下游请求格式。

这里必须区分输入和输出：

- **输入素材**：只有明确使用 `attachment://N` 的本地文件会被上传；下游提交的 `http://` 或 `https://` URL 原样进入上游请求。
- **生成结果**：上游返回的视频 URL 不会直接暴露给下游。Sub2API 会先下载到共享文件存储，再把自己的 `/media/{id}/asset.mp4` 或 `/media/{id}/asset.mov` URL 写入任务结果。

## 1. 基础信息

假设 Sub2API 地址为：

```text
https://sub2api.example.com
```

视频接口：

```text
POST /v1/videos
GET  /v1/videos/{id}
```

本地素材随视频请求提交：

```text
POST /v1/videos (multipart/form-data)
```

任务结果媒体读取：

```text
GET  /media/{id}/{filename}
HEAD /media/{id}/{filename}
```

所有需要认证的请求使用：

```http
Authorization: Bearer <API_KEY>
```

## 2. API Key 权限与参考素材上传

客户端可先将本地参考素材上传到 Agent 专属接口，再将返回的公网 URL 作为 JSON 参考素材提交给 `POST /v1/videos`：

```text
POST /api/v1/agent/assets
```

该接口使用 `Authorization: Bearer <AGENT_API_KEY>`，且只接受 Agent 分组 API Key。普通 API Key 返回 `403 agent_credential_required`。请求为 `multipart/form-data`，文件字段固定为 `file`；成功时返回 `201`，包含 `id`、`url`、`content_type`、`size`、`sha256`、`metadata` 与 `expires_at`。

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/api/v1/agent/assets' \
  -H 'Authorization: Bearer <AGENT_API_KEY>' \
  -F 'file=@./reference.png'
```

返回的 `url` 是由 Sub2API 托管的临时公网地址，可直接用于后续 `/v1/videos` JSON 请求中的 `image_url`、`video_url` 或 `audio_url`。资产受 File Service 的保留时长和单个 API Key 的每日上传配额限制。

视频接口仍支持把本地文件和 `attachment://N` 一起作为 multipart 请求提交；两种方式共用相同的媒体检查、存储、配额和过期清理逻辑。

Agent 专属的下列接口保留 Agent 分组限制：

```text
GET  /api/v1/agent/pricing
POST /api/v1/agent/generation/estimates
GET  /api/v1/agent/generation/estimates/{id}
POST /api/v1/agent/assets
```

## 3. 公网 URL 配置

multipart 本地素材和视频生成结果使用共享文件服务的公网根地址。推荐在管理后台配置：

```text
Admin → File Service → Public Base URL
```

填写根地址：

```text
https://sub2api.example.com
```

不要填写：

```text
https://sub2api.example.com/media
https://sub2api.example.com/media/
```

也不要添加查询参数、路径、用户名或密码。系统会自动拼接 `/media/{id}/asset.{extension}`。

部署环境也可以使用环境变量作为启动回退配置：

```env
FILE_SERVICE_PUBLIC_BASE_URL=https://sub2api.example.com
FILE_SERVICE_RETENTION_HOURS=24
FILE_SERVICE_DAILY_MAX_COUNT=100
FILE_SERVICE_DAILY_MAX_BYTES=2147483648
```

管理后台中保存的 File Service 配置优先于环境变量，并且保存后立即生效。

如果后台和环境变量都没有配置公网根地址，服务会根据上传请求的 `Host` 和 `X-Forwarded-Proto` 推导地址。因此反向代理至少需要正确转发：

```http
Host: sub2api.example.com
X-Forwarded-Proto: https
```

生产环境建议在后台 `File Service` 中显式保存 `Public Base URL`，避免生成容器内部 Host 或 HTTP URL。在线升级部署只需修改后台配置，不需要修改 `.env` 或重启服务。

反向代理还必须把以下路径转发到 Sub2API API：

```text
/media/*
```

即使底层使用 S3、Cloudflare R2、MinIO 或其他对象存储，对外媒体地址仍使用 Sub2API 的 `/media/...` URL。Seedance 上游访问该 URL 时，Sub2API 会从配置的存储后端读取文件并响应。

视频生成结果也使用同一路由。下游只看到 Sub2API 域名，不会得到对象存储直链或 Seedance 供应商的原始结果 URL。

如果 API 服务与视频任务 worker 分开部署，或运行多个 Sub2API 实例：

- 使用 `local` 后端时，所有实例必须挂载同一份 `agent-assets` 目录。
- 无法共享本地目录时，应在 File Service 中配置 S3、Cloudflare R2、MinIO 等共享对象存储。
- 所有实例应连接同一配置数据库，并在后台保存统一的外部 HTTPS `Public Base URL`；环境变量仅作为数据库未配置时的启动回退。还需确保 `/media/*` 可由下游访问。

## 4. 模式一：已有公网 URL，直接提交

如果图片、视频和音频已经能被上游服务器通过 HTTPS 访问，直接使用 JSON 请求。

### 4.1 纯文本生成

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: seedance-text-001' \
  -d '{
    "model": "seedance-2.0",
    "prompt": "一条小船在雾中的湖面上缓慢前进，电影感镜头",
    "ability_code": "video_text_to_video",
    "aspect_ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "generate_audio": true
  }'
```

### 4.2 公网首帧图片

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "seedance-2.0",
    "prompt": "人物抬头看向天空，头发被微风吹动",
    "ability_code": "video_image_to_video",
    "aspect_ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "https://cdn.example.com/character-first-frame.png"
        }
      }
    ]
  }'
```

### 4.3 公网首尾帧图片

```json
{
  "model": "seedance-2.0",
  "prompt": "从首帧自然运动到尾帧，保持人物身份、服装和镜头方向一致",
  "ability_code": "video_start_end_to_video",
  "aspect_ratio": "16:9",
  "duration": 5,
  "resolution": "720p",
  "content": [
    {
      "type": "image_url",
      "role": "first_frame",
      "image_url": {
        "url": "https://cdn.example.com/first-frame.png"
      }
    },
    {
      "type": "image_url",
      "role": "last_frame",
      "image_url": {
        "url": "https://cdn.example.com/last-frame.png"
      }
    }
  ]
}
```

### 4.4 混合公网图片、视频和音频

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "seedance-2.0",
    "prompt": "参考人物、动作节奏和背景音乐生成新的镜头",
    "ability_code": "video_reference_to_video",
    "aspect_ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "generate_audio": true,
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "subject_type": "person",
        "image_url": {
          "url": "https://cdn.example.com/character.png"
        }
      },
      {
        "type": "video_url",
        "role": "reference_video",
        "subject_type": "person",
        "duration_seconds": 5,
        "video_url": {
          "url": "https://cdn.example.com/motion.mp4"
        }
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {
          "url": "https://cdn.example.com/music.mp3"
        }
      }
    ]
  }'
```

在这种模式下，URL 会保留在规范化请求中。Aigod/Jingyu/newtoken 适配器按各自的 URL 引用协议直接发送；YCYAPI 适配器会在上游边界读取素材并转换为 YCYAPI 要求的 multipart 文件字段。

newtoken 适配器的字段拆分规则：`role: "first_frame"` 和 `role: "last_frame"` 的图片分别写入同名的字符串字段，其余图片、视频、音频依次收集到 `extra_images`、`extra_videos`、`extra_audios` 三个 URL 数组。

## 5. 模式二：一次 multipart 请求上传本地文件

### 5.1 multipart 协议

请求使用：

```http
POST /v1/videos
Authorization: Bearer <API_KEY>
Content-Type: multipart/form-data; boundary=...
```

表单字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `request` | JSON 字符串 | 是 | 视频创建请求；也支持字段名 `json` 或 `body` |
| `file` | 文件，可重复 | 按引用 | 本地图片、视频、音频文件 |

文件引用格式：

```text
attachment://0  → 第 1 个 file 部件
attachment://1  → 第 2 个 file 部件
attachment://2  → 第 3 个 file 部件
```

`file` 字段的线序以 multipart 原始顺序为准。客户端不要依赖文件名排序，也不要把文件拆到不同字段名；需要保持稳定顺序时，重复使用同一个 `file` 字段。

### 5.2 完整 curl 示例：本地图片 + 本地视频 + 公网音频

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <API_KEY>' \
  -H 'Idempotency-Key: seedance-reference-001' \
  -F 'request={
    "model": "seedance-2.0",
    "prompt": "保持人物身份一致，参考视频中的动作节奏生成新镜头",
    "ability_code": "video_reference_to_video",
    "aspect_ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "generate_audio": true,
    "seed": 123,
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "subject_type": "person",
        "image_url": {
          "url": "attachment://0"
        }
      },
      {
        "type": "video_url",
        "role": "reference_video",
        "subject_type": "person",
        "duration_seconds": 5,
        "video_url": {
          "url": "attachment://1"
        }
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {
          "url": "https://cdn.example.com/background.mp3"
        }
      }
    ]
  };type=application/json' \
  -F 'file=@./character.png;type=image/png' \
  -F 'file=@./motion.mp4;type=video/mp4'
```

服务端内部处理结果等价于把请求体中的：

```json
{
  "url": "attachment://0"
}
```

改写为：

```json
{
  "url": "https://sub2api.example.com/media/<asset-id-0>/asset.png"
}
```

把：

```json
{
  "url": "attachment://1"
}
```

改写为：

```json
{
  "url": "https://sub2api.example.com/media/<asset-id-1>/asset.mp4"
}
```

公网音频 URL 保持：

```text
https://cdn.example.com/background.mp3
```

然后才进入现有视频服务和上游适配器。

### 5.3 Node.js 20+ 完整示例

Node.js 20+ 可以直接使用内置 `fetch`、`FormData`、`Blob`：

```js
import { readFile } from "node:fs/promises";

const baseURL = process.env.SUB2API_BASE_URL ?? "https://sub2api.example.com";
const apiKey = process.env.SUB2API_API_KEY;

if (!apiKey) {
  throw new Error("SUB2API_API_KEY is required");
}

const imageBytes = await readFile("./character.png");
const videoBytes = await readFile("./motion.mp4");

const request = {
  model: "seedance-2.0",
  prompt: "保持人物身份一致，参考视频动作生成新镜头",
  ability_code: "video_reference_to_video",
  aspect_ratio: "16:9",
  duration: 5,
  resolution: "720p",
  generate_audio: true,
  content: [
    {
      type: "image_url",
      role: "reference_image",
      subject_type: "person",
      image_url: { url: "attachment://0" }
    },
    {
      type: "video_url",
      role: "reference_video",
      subject_type: "person",
      duration_seconds: 5,
      video_url: { url: "attachment://1" }
    },
    {
      type: "audio_url",
      role: "reference_audio",
      audio_url: { url: "https://cdn.example.com/background.mp3" }
    }
  ]
};

const form = new FormData();
form.append("request", new Blob([JSON.stringify(request)], { type: "application/json" }));
form.append("file", new Blob([imageBytes], { type: "image/png" }), "character.png");
form.append("file", new Blob([videoBytes], { type: "video/mp4" }), "motion.mp4");

const response = await fetch(`${baseURL}/v1/videos`, {
  method: "POST",
  headers: {
    Authorization: `Bearer ${apiKey}`,
    "Idempotency-Key": "seedance-node-example-001"
  },
  body: form
});

const payload = await response.json();
if (!response.ok) {
  throw new Error(`${response.status}: ${JSON.stringify(payload)}`);
}

console.log(payload);
```

注意：不要手动设置 `Content-Type: multipart/form-data`，Node.js 会自动生成包含 boundary 的正确值。

### 5.4 Python 完整示例

依赖：

```bash
python3 -m pip install requests
```

代码：

```python
import json
import os
from pathlib import Path

import requests

base_url = os.environ.get("SUB2API_BASE_URL", "https://sub2api.example.com")
api_key = os.environ["SUB2API_API_KEY"]

request_body = {
    "model": "seedance-2.0",
    "prompt": "保持人物身份一致，参考视频动作生成新镜头",
    "ability_code": "video_reference_to_video",
    "aspect_ratio": "16:9",
    "duration": 5,
    "resolution": "720p",
    "generate_audio": True,
    "content": [
        {
            "type": "image_url",
            "role": "reference_image",
            "subject_type": "person",
            "image_url": {"url": "attachment://0"},
        },
        {
            "type": "video_url",
            "role": "reference_video",
            "subject_type": "person",
            "duration_seconds": 5,
            "video_url": {"url": "attachment://1"},
        },
        {
            "type": "audio_url",
            "role": "reference_audio",
            "audio_url": {"url": "https://cdn.example.com/background.mp3"},
        },
    ],
}

with Path("./character.png").open("rb") as image_file, Path("./motion.mp4").open("rb") as video_file:
    files = [
        ("file", ("character.png", image_file, "image/png")),
        ("file", ("motion.mp4", video_file, "video/mp4")),
    ]
    response = requests.post(
        f"{base_url}/v1/videos",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Idempotency-Key": "seedance-python-example-001",
        },
        data={"request": json.dumps(request_body)},
        files=files,
        timeout=120,
    )

response.raise_for_status()
print(json.dumps(response.json(), ensure_ascii=False, indent=2))
```

### 5.5 不同文件类型的完整顺序示例

下面的请求中有 4 个文件，文件顺序与引用关系如下：

| multipart 顺序 | 文件 | 请求体引用 |
| ---: | --- | --- |
| 0 | `first.png` | `attachment://0` |
| 1 | `last.png` | `attachment://1` |
| 2 | `motion.mp4` | `attachment://2` |
| 3 | `voice.mp3` | `attachment://3` |

```bash
curl -sS \
  -X POST 'https://sub2api.example.com/v1/videos' \
  -H 'Authorization: Bearer <API_KEY>' \
  -F 'request={
    "model":"seedance-2.0",
    "prompt":"从首帧运动到尾帧，并参考动作视频和音频",
    "ability_code":"video_reference_to_video",
    "duration":5,
    "resolution":"720p",
    "content":[
      {"type":"image_url","role":"first_frame","image_url":{"url":"attachment://0"}},
      {"type":"image_url","role":"last_frame","image_url":{"url":"attachment://1"}},
      {"type":"video_url","role":"reference_video","duration_seconds":5,"video_url":{"url":"attachment://2"}},
      {"type":"audio_url","role":"reference_audio","audio_url":{"url":"attachment://3"}}
    ]
  };type=application/json' \
  -F 'file=@./first.png;type=image/png' \
  -F 'file=@./last.png;type=image/png' \
  -F 'file=@./motion.mp4;type=video/mp4' \
  -F 'file=@./voice.mp3;type=audio/mpeg'
```

如果 `first.png` 和 `last.png` 的顺序颠倒，Sub2API 也会按照实际 multipart 顺序生成 URL；因此必须同时调整 `attachment://N` 的引用，不能依赖文件名自动排序。

## 6. 视频任务响应和查询

创建视频任务成功后返回任务 ID：

```json
{
  "id": "video_0123456789abcdef",
  "object": "video",
  "model": "seedance-2.0",
  "status": "queued",
  "refund_status": "not-applicable",
  "created_at": 1784102400
}
```

轮询：

```bash
curl -sS \
  'https://sub2api.example.com/v1/videos/video_0123456789abcdef' \
  -H 'Authorization: Bearer <API_KEY>'
```

完成响应示例：

```json
{
  "id": "video_0123456789abcdef",
  "object": "video",
  "model": "seedance-2.0",
  "status": "completed",
  "video_url": "https://sub2api.example.com/media/8db0d973-c281-4b6e-a6d7-550f2bcc2b31/asset.mp4",
  "refund_status": "not-applicable",
  "created_at": 1784102400,
  "completed_at": 1784102475
}
```

任务查询需要使用提交任务时的同一个 API Key。

### 6.1 生成结果转存流程

当上游任务报告 `completed` 时，Sub2API 不会立即把供应商 URL 写入任务。实际流程如下：

```text
上游 completed + 原始视频 URL
  → Sub2API 使用服务端 HTTP 客户端流式下载
  → 校验 MP4/QuickTime 的 ISO-BMFF 文件头和 200 MiB 大小上限
  → 写入 File Service 配置的 local 或 S3/R2 存储
  → 在 temporary_assets 中记录生命周期
  → 将 https://sub2api.example.com/media/{id}/asset.mp4 写入 video_tasks
  → 下游轮询得到 completed
```

安全和可见性规则：

- 上游原始 URL 只用于服务端下载，不写入 `video_tasks.result_video_url`、`temporary_assets.metadata` 或 usage log。
- 下游创建响应、轮询响应和完成后的 usage log 都只包含 Sub2API URL。
- 下载会拒绝非 HTTP(S) 地址、内网/loopback/link-local/云元数据目标、非视频内容以及超过 200 MiB 的结果。
- 支持 `video/mp4` 和 `video/quicktime`；返回扩展名分别为 `.mp4` 和 `.mov`。
- 转存尚未完成时，任务保持 `processing`，下游不会看到临时的供应商 URL。
- 下载、存储或数据库记录失败时，生命周期轮询会继续重试；如果超过该上游账号配置的总轮询超时，任务按现有失败和退款流程处理，并返回脱敏错误。
- 生成结果沿用 File Service 的 `Retention Hours`、每日文件数量和每日总字节配额。过期后由现有临时资产清理机制删除。

`/media/*` 路由支持 `GET`、`HEAD` 和 Range 请求，播放器和下载客户端应直接使用返回的 `video_url`。

### 6.2 Jingyu 视频任务使用完成回调

此调整**只作用于后台账号配置中 `video_provider = jingyu` 的视频任务**：

- Sub2API 向 Jingyu `POST /v1/video/generations` 创建任务时，会自动增加 `callback_url` 和 `callback_secret`。
- `callback_url` 形如：

  ```text
  https://sub2api.example.com/api/v1/webhooks/jingyu/videos/{sub2api-video-id}
  ```

- `callback_secret` 由 Sub2API 使用服务端 JWT secret 和当前视频任务 ID 派生，每个任务不同，不会写入任务查询响应。
- Jingyu 回调请求必须包含：

  ```http
  X-NewAPI-Event: task.completed
  X-NewAPI-Signature: <hex hmac sha256>
  ```

- Sub2API 使用原始 HTTP 请求体校验 `hex(HMAC-SHA256(callback_secret, raw_body))`，不会对 JSON 重新序列化后再验签。
- 成功回调中的 `video_url`、`result_url` 或 `result_asset_url` 仍需先经过第 6.1 节的结果转存，完成后任务才变为 `completed`。
- 失败回调会把任务变为 `failed` 并执行现有退款流程；重复回调按任务终态幂等处理。
- Jingyu 创建成功后，Sub2API 不再向 Jingyu 发送任务状态 GET 轮询。原 `poll_timeout_ms` 只作为等待回调的总超时看门狗；到期仍未收到有效终态回调时，任务失败并退款。

范围边界：

- Aigod 视频任务继续使用原有上游状态轮询。
- 下游客户端仍然通过 `GET /v1/videos/{id}` 查询 Sub2API 任务状态，不需要接收 Jingyu 回调。
- OpenAI 图片、异步图片、批量图片以及其他图片接口没有接入此回调路由，行为保持不变。

部署要求：Jingyu 必须能从公网访问上述 HTTPS 回调地址。推荐在后台 `Admin -> File Service -> Public Base URL` 保存 `https://sub2api.example.com`，无需修改 `.env`；同时在反向代理或 WAF 中放行：

```text
POST /api/v1/webhooks/jingyu/videos/*
```

该路由不使用下游 API Key 鉴权，只接受每任务 HMAC 验签通过的 Jingyu 视频终态消息。回调处理返回任意 `2xx` 时 Jingyu 视为投递成功；验签或请求体无效返回 `4xx`，临时存储、下载或数据库处理失败返回 `5xx`，以触发 Jingyu 文档中的重试策略。

## 7. Seedance 能力和输入规则

### 7.1 能力代码

```text
video_text_to_video       纯文本生成
video_image_to_video      单首帧图生视频
video_start_end_to_video  首尾帧生成视频
video_reference_to_video  参考图片/视频/音频生成视频
```

没有显式传 `ability_code` 时，Sub2API 会根据 `content` 中的媒体类型进行推断；生产客户端建议显式传递，避免请求意图不清晰。

### 7.2 图生视频

必须恰好包含一张：

```json
{
  "type": "image_url",
  "role": "first_frame",
  "image_url": {"url": "https://..."}
}
```

不能同时加入视频或音频引用。

### 7.3 首尾帧

必须包含：

- 一张 `role: "first_frame"` 图片。
- 一张 `role: "last_frame"` 图片。

不能同时加入视频或音频引用。

### 7.4 参考视频

Seedance 2.0 参考模式支持图片、视频和音频，但至少需要一张图片或一个视频。视频引用需要：

```json
{
  "type": "video_url",
  "role": "reference_video",
  "duration_seconds": 5,
  "video_url": {"url": "https://..."}
}
```

当前代码校验的常用限制：

| 模型 | 单次时长 | 图片数量 | 视频数量 | 音频数量 | 总引用数量 |
| --- | ---: | ---: | ---: | ---: | ---: |
| `seedance-2.0` | 4–15 秒 | 最多 9 | 最多 3 | 最多 3 | 按图片/视频规则校验 |
| `seedance-2.0-fast` | 4–15 秒 | 按上游能力 | 按上游能力 | 按上游能力 | 按上游能力 |
| `seedance-2.5` | 4–30 秒 | 最多 30 | 最多 10 | 最多 10 | 最多 50 |

最终可用分辨率仍取决于模型和已配置的上游路由：

```text
seedance-2.0       480p / 720p / 1080p / 4K
seedance-2.0-fast  480p / 720p
seedance-2.5       480p / 720p
```

newtoken 上游把输出分辨率编码进了模型 ID 本身，因此该上游只按下面的组合路由，下游请求格式不变：

| 下游 `model` | `resolution` | newtoken 上游模型 |
| --- | --- | --- |
| `seedance-2.0` | `720p` | `sd2.0-720p-official` |
| `seedance-2.0` | `1080p` | `sd2.0-1080p-official` |
| `seedance-2.0-fast` | `720p` | `sd2.0-720p-fast-official` |
| `seedance-2.5` | `720p` | `sd2.5-720p-official` |

表外的组合（例如任何 `480p` 请求、`seedance-2.0` 的 `4K`）在 newtoken 上没有对应模型，调度时会跳过 newtoken 账号并回落到其他已配置的上游；只有在没有任何上游支持该组合时才会返回错误。

## 8. 支持的上传格式和大小

| 媒体 | 推荐格式 | 当前支持格式 | 单文件上限 |
| --- | --- | --- | ---: |
| 图片 | PNG、JPEG、WebP | JPEG、PNG、WebP、GIF、BMP、TIFF、HEIC、HEIF | 30 MiB |
| 视频 | MP4/H.264 | MP4、MOV | 200 MiB |
| 音频 | MP3、WAV | MP3、WAV | 15 MiB |

建议视频优先使用 MP4/H.264。部分上游会依赖文件扩展名，MOV、HEIC 等格式虽然可以上传，但统一转为 PNG/JPEG/MP4 后兼容性更好。

## 9. 错误响应

错误统一使用：

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "invalid_attachment_reference",
    "message": "Content item 2 references missing attachment 3"
  }
}
```

常见错误码：

| HTTP | code | 原因 |
| ---: | --- | --- |
| 400 | `video_request_part_required` | multipart 请求没有 `request`、`json` 或 `body` JSON 字段 |
| 400 | `invalid_video_multipart` | multipart 解析失败 |
| 400 | `invalid_attachment_reference` | `attachment://N` 格式错误 |
| 400 | `attachment_reference_out_of_range` | N 超出本次请求的 file 数量 |
| 400 | `unsupported_media` | 文件类型、扩展名或内容不匹配 |
| 413 | `media_too_large` | 超过图片、视频或音频单文件限制 |
| 413 | `request_body_too_large` | 整个 HTTP 请求超过网关限制 |
| 422 | `media_probe_failed` | 图片、视频或音频无法被可信探测器解析 |
| 429 | `temporary_asset_quota_exceeded` | API Key 24 小时上传数量或总字节数超限 |
| 503 | `media_upload_unavailable` | 视频 Handler 没有连接到共享素材存储 |
| 503 | `file_storage_unavailable` | 文件存储配置或对象存储不可用 |

## 10. 重试建议

本地文件只能随一次性 multipart 请求提交。收到明确的 4xx 校验错误时，修正请求后再提交；遇到网络超时或连接中断时，任务是否已经创建可能不明确，不要自动重复 POST，以免产生重复任务和重复预扣费。

需要复用素材时，应由客户端放在自己的公网对象存储或 CDN，然后使用普通 JSON 请求提交该公网 URL。Sub2API 会保持现有 `http://` 或 `https://` 输入地址不变。

## 11. 客户端实现要点

1. 永远按照 multipart 文件添加顺序生成 `attachment://N`。
2. 不要按文件名排序后再提交，除非请求体引用也同步重排。
3. 已经是公网 `http(s)` URL 的媒体直接写入 JSON，不要包装成 `attachment://N`。
4. 引用视频的 `duration_seconds` 由客户端在提交前探测，例如使用 `ffprobe` 读取真实时长。
5. 公网 URL 必须允许 Seedance 上游服务通过无认证 HTTPS `GET` 访问。
6. 保存视频任务 `id`，并使用创建任务时的同一 API Key 查询状态。
7. `completed` 后及时下载并按业务需要持久化 `video_url`。
