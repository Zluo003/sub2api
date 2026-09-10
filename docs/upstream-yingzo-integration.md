# Upstream-first Yingzo integration

Branch: `codex/upstream-base-yingzo-20260910`

This branch starts at `upstream/main` (`98d86915b`, Sub2API 0.2.4). The existing
二开 `main` branch remains unchanged and is tagged as
`codex-pre-upstream-20260910` for rollback. The upstream base is tagged as
`codex-upstream-base-20260910`.

The integration keeps upstream standard protocol handlers and services for
Gemini, Claude, OpenAI, model mapping, retries, and error handling. Yingzo
extensions are added as separate modules and provider bindings:

- Seedance/video handlers, tasks, providers, pricing rules, usage settlement,
  result rehosting, and migrations 156–160 and 168.
- Agent groups, model catalogue, pricing, account pool, asset transport, and
  `/v1/agent/*` routes, including migrations 161–177 and 197.
- Temporary assets, file storage, object-store adapters, cache/lease metadata,
  and video reference-file linkage.

Ent schema and generated code were regenerated after adding the custom group,
user, video, and monitor fields. Wire was regenerated so both standard and
Yingzo providers are discoverable at startup.

Validation performed:

- `go build ./...` in `backend`: passes.
- Targeted production packages (`internal/service`, `internal/handler`,
  `internal/server/routes`): pass with `go build`.
- `go test` is not green yet because the tree contains mixed-era test doubles:
  older Yingzo tests expect pre-upstream method signatures and a few route test
  helpers that are absent from the upstream route files. These are test-only
  compile failures; production compilation is clean and the failing symbols
  are listed in the integration task log.

No upstream standard Gemini/Claude/OpenAI request transformer was replaced by
the older二开 implementation in this branch.
