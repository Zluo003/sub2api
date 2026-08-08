# Upstream merge: Wei-Shaw/sub2api v0.1.165 → fork

Integration branch: `merge-upstream-v0.1.165`; all conflicts are resolved and verified for fork release `v0.1.166`.

## Baseline discovery

The fork has **no shared git ancestry** with upstream — it was created from a squashed
`Initial commit` (bc56ad42, 2026-06-29). The true upstream base was identified as:

    82553c4dc  2026-06-28 11:13:09 +0800  fix(openai): preserve quota platform in usage billing

Evidence: every file touched by 82553c4dc matches the fork's initial tree byte-for-byte
(5/6; the 6th, `openai_gateway_service.go`, is fork-modified). Every upstream commit after
it (d86e83259, 709cf6185, 7cbf82ed6, 4a7148e20, 10e623f67) has 0/N files matching.

A local graft makes merge-base computation work:

    git replace --graft bc56ad4255123d6db4c6957cdc9b0d94f94917f1 82553c4dc

The graft is only needed for **this** merge. The resulting merge commit records
`upstream/main` as a real second parent, so future merges get a proper base automatically.
Upstream tags are fetched into `refs/tags/upstream/*` to avoid clobbering the fork's own
`v0.1.x` tags.

Scale: 1225 upstream commits, 1600 files, +318k/-68k. Fork surface: 345 files, +42k/-4.7k.

## Fork (二开) surface to preserve

- **Video platform / Seedance**: `/v1/videos` async API, `video_handler.go`, `video_service.go`,
  `video_provider_{adapter,aigod,jingyu}.go`, `video_repo.go`, ent `video_task` +
  `video_group_pricing_rule`.
- **Yingzo agent gateway**: `agent_handler.go`, `agent_pricing*.go`, `agent_models.go`,
  `agent_channel_pricing.go`, `agent_group_context.go`, `routes/agent.go`.
- **File storage service** (S3/R2): `file_storage_service.go`, `admin/file_storage_handler.go`,
  `temporary_asset_publisher.go`; needs `github.com/aws/smithy-go`.
- **Frontend**: `ModelPlazaView`, `FileServiceView`, `ApiDocsView`, `AgentModelCatalogEditor`,
  `utils/modelCatalog.ts`, `utils/videoUsage.ts`, `api/fileService.ts`.
- **Migrations 154–177** (fork-numbered).

## Resolved design decisions

### 1. `usage_logs` video columns — both sides added overlapping columns

Fork migration `156_video_gateway.sql` sorts before upstream `172_video_per_second_billing_metadata.sql`,
and both use `ADD COLUMN IF NOT EXISTS`, so the **fork's definitions win** on every database:

| column | effective definition | added by |
|---|---|---|
| `video_resolution` | `VARCHAR(16)` | fork 156 (upstream 172's `VARCHAR(10)` is a no-op) |
| `video_duration_seconds` | `INTEGER NOT NULL DEFAULT 0` | fork 156 (upstream 172's nullable variant is a no-op) |
| `video_task_id` | `VARCHAR(64)` | fork 156 |
| `video_reference_duration_seconds`, `video_billable_seconds`, `video_result_url` | | fork 156 |
| `video_count` | `INTEGER NOT NULL DEFAULT 0` | upstream 172 |

**Decision**: ent schema declares the union, with `video_duration_seconds` as non-null
`field.Int(...).Default(0)` (Go `int`), matching the real column. Upstream's Grok-video code
declared it `*int` and writes `nullInt(...)`; writing NULL into a `NOT NULL` column would fail,
so **upstream's `*int` usages are adapted to plain `int`**. Upstream does not rely on the
nullability for semantics — it marks video rows with `video_count > 0`.

Sites to adapt: `service/usage_log.go`, `repository/usage_log_repo_insert.go`,
`repository/usage_log_repo_query.go`, `service/openai_gateway_usage.go`, `service/grok_media.go`.

### 1b. `RequestType = 5` collision — fork keeps 5, upstream's `live` moves to 6

Fork defines `RequestTypeVideo = 5`; upstream added `RequestTypeLive = 5`. The same integer
cannot carry both meanings, and this fork's `usage_logs` **already has `request_type = 5` rows
meaning video**. So `RequestTypeVideo` keeps 5 and `RequestTypeLive` is remapped to **6**
(`internal/service/usage_log.go`). New migration `191_allow_live_request_type_six.sql` widens
`usage_logs_request_type_check` from `0..5` to `0..6` (superset; existing rows validate instantly).
`ParseUsageRequestType`/`String()`/`IsValid()` accept both `video` and `live`.
Frontend `utils/usageRequestType.ts` likewise accepts both.

### 1c. `BillingModeVideo` collision — two distinct billing modes

Fork defined `BillingModeVideo = "video_duration"` (Seedance per-second); upstream defines
`BillingModeVideo = "video"` (Grok per-video). Both values exist in `usage_logs.billing_mode`,
so they are kept as **two constants** in `internal/service/channel.go`:
`BillingModeVideo = "video"` (upstream's meaning, name preserved for upstream call sites) and
`BillingModeVideoDuration = "video_duration"`. The fork's call sites in `video_service.go`
were repointed to `BillingModeVideoDuration`; both are accepted by `IsValid`/`IsValidUsageFilter`.
`repository/usage_log_repo.go`'s billing-mode filter needs no change — its `case BillingModeVideo`
and `default` branches emit identical SQL, so `video_duration` still filters exactly.

### 2. CHECK-constraint chains — verified safe, no action needed

Migrations run sorted by filename; last writer wins.

- `usage_logs_request_type_check`: 061 → **158 (fork, 0–5)** → 173 (upstream, 0–4) → **188 (upstream, `>=0 AND <=5`)**.
  Final allows the fork's `request_type=5` (video). OK.
- `user_platform_quotas_platform_check`: 157 (upstream, adds `grok`) → 158 (fork) → **168 (fork, final)**
  = `('anthropic','openai','gemini','antigravity','grok','seedance')`. Retains upstream's `grok`. OK.
- `usage_logs_image_billing_size_check`: 136 → 171 → **172 (upstream, final)**. Fork video rows
  use `billing_mode='video_duration'` with `image_count=0`, satisfying the `image_count <= 0` branch. OK.

### 3. Upstream refactors the fork must follow

- **`usage_log_repo.go` was split** into `usage_log_repo.go` + `_insert.go` + `_query.go` +
  `_stats.go` + `_trend.go` + `_dashboard.go`. Fork video insert/query code must be ported
  into the new files, not left in the old one.
- **i18n was modularized**: `locales/en.ts`/`zh.ts` → `locales/{en,zh}/**` (common, dashboard,
  landing, misc, batchImage, admin/*). The fork's added keys must be ported into the new modules.
  Fork key groups: `nav.{fileService,modelPlaza,apiDocs}`, `apiDocs`, `modelPlaza`,
  `usage.video*`, `admin.groups.agent*`, `admin.groups.videoPricing`, platform `seedance`,
  `billingModeVideo`.

### 4. Generated code

`backend/ent/*` is regenerated (`cd backend && go generate ./ent`) from the merged schema
rather than hand-merged. Requires `backend/internal/domain` to compile first.
`go.sum` is regenerated via `go mod tidy`.

`backend/go.mod`: took the max of each conflicting version pair and kept the fork's
`github.com/aws/smithy-go v1.24.2`.

## 状态

**合并完成**：后端、前端、文档与部署配置冲突均已解决，完整验证结果见下文。

## 上游的大规模文件拆分（本次合并最大的坑）

上游把 5 个巨型文件拆成了多文件，冲突块里上游侧是**空的**：

| 文件 | 行数 | 拆成 |
|---|---|---|
| `gateway_service.go` | 10685→1397 | `gateway_scheduling.go`、`gateway_usage_billing.go` 等 |
| `openai_gateway_service.go` | 7604→1183 | `openai_gateway_usage.go`、`openai_gateway_scheduling.go`、`openai_gateway_forward.go` 等 |
| `openai_ws_forwarder.go` | 4588→391 | `openai_ws_forwarder_ingress.go` 等 |
| `usage_log_repo.go` | 4642→224 | `_insert.go`/`_query.go`/`_stats.go`/`_trend.go`/`_dashboard.go` |
| `admin_service.go` | 3998→697 | `admin_group.go`/`admin_account.go`/`admin_proxy.go` |

**两个反向陷阱**：
1. 对这些文件用 `both` 求并集 → 上游已迁走的函数被留下 → **重复定义**编译失败（噪音大但安全）。
2. 用 `theirs` 取上游 → **能编译通过，但二开功能被静默删除**（危险）。

因此每个拆分文件都必须：取上游版本 → 逐条比对 fork 的 diff → 把二开改动移植到新文件。
用下面这条命令可以查出被静默删掉的函数：

    for m in $(git show HEAD:backend/<file> | grep -oE '^func (\([a-z] \*[A-Za-z]+\) )?[A-Za-z0-9_]+' | sed -E 's/^func (\([a-z] \*[A-Za-z]+\) )?//' | sort -u); do
      grep -rqE "^func (\([a-z] \*[A-Za-z]+\) )?$m\(" backend/internal/service/ || echo "MISSING: $m"
    done

实际靠它捞回来的：`calculateAgentRecordUsageCost`、`hasAgentGroup`、`UpdateVideoResult`，
以及 `calculateOpenAIRecordUsageCost` 里整段的 Agent 图片/token 计费分支
（该函数上游还删掉了 `account` 参数，移植时需把它加回并在调用点传 `billingAccount`）。

## 已完成的二开移植

| 二开能力 | 移植到 |
|---|---|
| Agent 分组账号路由（Gateway/OpenAI 两条） | `gateway_scheduling.go`、`openai_gateway_scheduling.go` |
| Agent 图片/token 计费分支 | `openai_gateway_usage.go`（恢复 `account` 参数） |
| `calculateAgentRecordUsageCost` | `gateway_usage_billing.go` |
| 退款计费 `!= 0`、`shouldApply*Command` | `gateway_usage_billing.go` |
| `deferredService` 的 nil 保护（视频链路无此依赖） | `gateway_usage_billing.go` |
| `billingDeps.accountRepo` 收窄为 `AccountQuotaUsageRepository` | `gateway_usage_billing.go` |
| 视频定价 hydrate/normalize + Create/Update/DeleteGroup 逻辑 | `admin_group.go` |
| `validateAgentAccountGroupBindings`、`hasAgentGroup` | `admin_account.go` |
| `UpdateVideoResult` + 4 个视频列的 insert/scan 全链路 | `usage_log_repo_insert.go`/`_query.go` |
| `IsGeminiImageGenerationModel`/`GeminiImageBillingTier` | `antigravity_gateway_service.go` |

## 其它需要判断的冲突

- **路由撞车**：fork 的 `GET /v1/videos/:id`（Seedance）与上游的 `GET /videos/:request_id`（Grok）
  是同一路径不同参数名，gin 启动会 panic。统一注册为 `:id` + `videoGetDispatch` 按平台分发，
  并让 `grok_media.go` 的 `grokVideoRequestID()` 兼容两种参数名。
- **重复注册**：并集会把上游取代掉的旧 `r.POST(...)` 一并留下，gin 报
  "handlers are already registered"。已清理 `/embeddings`、`/images/*`、`/responses`、
  `/messages/count_tokens`（后者把 fork 的 Agent 分支折进了上游的 `countTokensHandler`）。
- **上游删除了 `rejectGrokUnsupportedEndpoint`**：上游现在支持 Grok 走 messages/responses，
  fork 的三处拒绝守卫已移除（保留 `setAgentRequestPlatform` 的 Agent 路由），
  相应的 fork 测试 `TestGatewayRoutesGrokOnlyAllowsResponsesHTTP` 已删除。
- **`upstream_endpoint` 不再暴露给普通用户**（上游只在 admin DTO 里赋值），
  并集误加到了 user DTO，已移除。
- **`/videos` 根别名归一化**：上游只匹配 `/videos/`，补了 `HasSuffix(path, "/videos")`。
- **prompt audit 覆盖**：上游新增测试要求每个 POST 网关路由都被分类。已把审核协调器
  接入 `VideoHandler`（新增 `ContentModerationProtocolVideos` + `ProvideVideoHandler`），
  `/v1/videos`、`/videos` 登记进 manifest。

## 前端

- **i18n 模块化**：上游把 `locales/{en,zh}.ts` 拆成了模块目录。二开新增的键（en 127 / zh 126）
  没有硬塞进上游模块，而是**结构化 diff 后单独生成 `locales/{en,zh}/fork.ts`**，
  由新增的 `i18n/deepMerge.ts` 在 `index.ts` 里深合并。这样上游以后再改各模块都不会冲突。
  上游自带的 8 个 i18n 测试（含键冲突检测、消息编译）全部通过。
- **计费模式/请求类型**：与后端同步，`BILLING_MODE_VIDEO='video'` 与
  `BILLING_MODE_VIDEO_DURATION='video_duration'` 并存；`UsageRequestType` 同时含 `video` 与 `live`；
  `GroupPlatform` / `Platform` 同时含 `seedance` 与 `composite`。
- **UsageView 被上游重构**：原先内联的 `<DataTable>` + 约 360 行单元格模板被抽成了共享的
  `components/admin/usage/UsageTable.vue`。二开的视频列渲染本就在这个共享组件里，
  所以改用上游结构不会丢功能（一开始误按 "保留 fork" 处理，导致 `<template>` 不闭合）。
- **上游修的真实 bug 已采纳**：`StripePopupView` 轮询读错 token 键（`token` → `auth_token`）、
  `KeysView` 列设置版本迁移、`PaymentView` 订阅按管理员配置的 USD→CNY 汇率换算
  （与后端 `calculateSubscriptionGatewayBaseAmount` 对齐，fork 原先按余额倍率换算的实现与测试已废弃）。
- **文档保持二开版本**：`README*.md`、`deploy/README.md` 取 fork 版（上游的营销版 README 未合入）。
- **`.github/audit-exceptions.yml`** 保留 fork 更严格的豁免理由与更早的到期日（2026-08-20，
  而非上游的 2026-10-06）。
- **pnpm 固定在 10.23.0**（上游为 9.15.9）：本仓库的 lock/workspace 由 pnpm 10 生成；
  根 `Dockerfile` 同时采纳上游的 store 缓存挂载并补拷 `pnpm-workspace.yaml`。

## 上游自身的两处问题（已在本仓库修正）

1. `api/__tests__/admin.system.rollback.spec.ts` 在上游就是**红的**：上游提交 `35b5edb24`
   给回退请求加了 `{ timeout }` 第三个参数，却没改这个断言。两个文件与上游逐字节一致，
   已把断言改为 `expect.objectContaining({ timeout: ... })`。
2. `GroupsView` 的列设置与复制测试最初产生 10 条 unhandled rejection：上游的
   `loadLiveCapability()` 调了测试里未打桩的 `adminAPI.groups.getLiveCapability`，导致
   断言虽然通过但 Vitest 仍以 1 退出。两个测试现已补齐能力接口桩，完整套件正常退出。

## 验证结果

- 后端：`go build ./...`、`go vet ./...` 全绿；`go test ./...` **45/45 包通过**。
- 前端：`vue-tsc --noEmit` **0 错误**；`vitest run` **193/193 文件、1325/1325 用例通过**；
  `vite build` 成功。

## 完成状态

- 上游合并、二开能力移植、前后端冲突、文档与部署配置均已完成并通过发布前验证。

## 工具

冲突批处理脚本 `resolve.py`（`ours|theirs|both` + 可选 1-based hunk 序号）在会话 scratchpad。
**注意**：跨越函数体的 hunk 用 `both` 会把两个函数体拼接在一起，每批之后必须跑 `gofmt -l`。
测试文件的通用套路是「`theirs` + 用 `comm` 找出 fork 独有的 `func Test*` 再 `awk` 追加回去」。
wire 需要单独装：`go install github.com/google/wire/cmd/wire@v0.7.0`（临时模块里装，别污染 go.mod）。

---

# Upstream merge: fork v0.1.167 + Wei-Shaw/sub2api v0.1.172

Integration branch: `codex/merge-upstream-v0.1.172`. The merge target is the upstream
stable tag `v0.1.172` (`155c494964c3ea6ecc31f52679525c1034bf0f16`), rather than the
one additional version-only commit currently on upstream `main`. The merged fork version is
`v0.1.173`.

## Imported upstream functionality

- Panel API rate limiting, passkey authentication, and the public model plaza.
- Kimi K3 and Gemini 3.6 Flash support, composite reasoning-effort policies, and Codex
  client-version synchronization with normalized outbound identity.
- Group-level profit controls plus cross-platform upstream billing-rate probing and optional
  writeback.
- Tencent and Aliyun CAPTCHA providers.
- Upstream response-model audit fields and mismatch filtering.
- OAuth account-takeover and upstream URL path-validation security fixes, plus billing,
  failover, WebSocket, payment, SMTP, proxy, and usage-statistics fixes from releases
  `v0.1.166` through `v0.1.172`.

## Fork compatibility decisions

- Both the fork's `AgentHandler` and upstream's `ModelPlazaHandler` remain registered; both
  Agent routes and upstream panel rate limiting are active.
- Admin group DTOs retain fork fields (`kind`, `system_code`) alongside upstream profit-control
  fields. Ordinary groups use request-level `PricingAt` for peak pricing; Agent groups keep
  their own multi-platform/model pricing and do not apply ordinary profit controls.
- Seedance asynchronous `/v1/videos`, `VideoService`, video tasks and pricing rules, S3/R2 file
  service, Yingzo Agent routing, and fork Model Plaza/File Service/API Docs pages are preserved.
- `RequestTypeVideo` stays `5` for existing fork data; upstream Live stays `6`.
  `BillingModeVideoDuration` remains distinct from upstream per-video billing.
- API-key auth snapshot version is `19`, covering both upstream profit-control data and fork
  Agent permissions.
- `usage_logs` is the union of upstream response-model audit fields and fork video fields. The
  fork's effective database definitions remain `video_resolution VARCHAR(16)` and
  `video_duration_seconds INTEGER NOT NULL DEFAULT 0`.
- Existing fork migrations and platform constraints continue to support `seedance` and `grok`.
  Fork README files remain authoritative.
- Ent and Wire output was regenerated from the merged source rather than hand-edited.

## Browser-discovered regression fix

The system Agent uses `openai` as its compatibility platform, which initially made upstream's
profit-control form appear in the Agent editor. `GroupsView.vue` now hides that form for
`kind === 'agent'` and omits all three profit-control fields from Agent update payloads. A
source regression test covers the display and payload guards.

Runtime verification confirms the intended separation:

- Yingzo Agent: model catalog and multi-platform pricing present; profit control absent.
- Ordinary OpenAI group: profit control present.
- Seedance group: video pricing present, including 4K and Fast variants.

## Verification

- Backend: `go test ./...` passed, including focused handler/service runs; Ent and Wire
  regeneration succeeded.
- Frontend: typecheck and lint passed; production build passed; Vitest passed all 213 files and
  1499 tests. The final focused group-layout/profit tests passed 17/17.
- Source hygiene: all merge conflicts resolved and `git diff --check` passed.
- Runtime: the source-built Docker image started with PostgreSQL and Redis healthy;
  `/health` returned `{"status":"ok"}` and `/` returned HTTP 200. Migrations 191-195 applied,
  the runtime reported `v0.1.173`, and persisted fork constraints were verified.
- Browser: the authenticated admin flow and all three group editors above were exercised
  against the rebuilt image at `http://127.0.0.1:8080`; browser diagnostic logs were empty.
