# 方案：请求事件日志新增「错误」列

## 1. 目标

在「Request Events / 请求事件日志」表格中新增一列「错误」：

- 当请求失败时，展示**错误短码**（如 `402`）和**原因**。
- 当请求成功时，该列显示空或 `—`。
- 列默认位置在 **Result 列之后**；同时纳入现有列设置，可自定义顺序/显隐。
- 历史失败事件没有错误码/原因数据，**不做回填**，旧数据该列显示 `—`，新数据从上线后开始填充。

本次只输出方案，不改代码。

---

## 2. 现状分析

### 2.1 前端

- 列定义：`web/src/components/usage/requestEventColumns.ts`
  - `REQUEST_EVENT_COLUMN_IDS` 当前包含 `result`，没有错误列。
- 表格渲染：`web/src/components/usage/RequestEventsDetailsCard.tsx`
  - 列渲染集中在 `columns` 配置数组中，`result` 列当前只根据 `event.failed` 显示 `Success` / `Failure`。
  - 行数据来自 `UsageEvent`。
- 列设置：`web/src/components/usage/RequestEventsColumnSettingsModal.tsx`
  - 基于 `REQUEST_EVENT_COLUMN_IDS` 自动支持显隐与拖拽排序。
- 用户偏好：`web/src/pages/UsagePage.tsx`
  - `REQUEST_EVENTS_PREFERENCES_VERSION = 8`，保存/恢复列顺序和可见性。
- 类型：`web/src/lib/types.ts`
  - `UsageEvent` 当前有 `failed: boolean`，没有 `error_code` / `error_message`。

### 2.2 后端

- `usage_events` 表当前只有 `failed` 布尔字段，没有错误码/原因列。
- CPA Redis 队列消息解码在 `internal/service/redis_usage.go`：
  - `queuedUsageDetail` 当前没有错误码/原因字段。
- 事件 API：`internal/api/usage_events.go`
  - `usageEventPayload` 当前没有错误码/原因字段。
- 归档表 `usage_events_archive` 与 `usage_events` 结构一致，后续新增列时也要同步。
- 插入方式：`repository.InsertUsageEvents` 使用 GORM `Create`，新增实体字段后会自动写入。

---

## 3. 总体设计

### 3.1 数据流

```text
CPA Redis usage 消息（失败时带 error_code / error_message）
   │  service.redis_usage 解码新增字段
   ▼
usage_events 表新增列 error_code / error_message（历史行为 NULL）
   │  API 查询映射
   ▼
GET /api/v1/usage/events → 每个 event 增加 error_code / error_message
   │
   ▼
前端 UsageEvent 类型新增字段
   │
   ▼
RequestEventsDetailsCard 新增「错误」列：
   - failed 且 error_code 存在 → 展示短码 + 原因
   - failed 但无错误码（历史数据）→ 展示 `—`
   - 成功 → 展示 `—`
```

### 3.2 关键决策

| 决策点 | 结论 |
|---|---|
| 错误码/原因来源 | CPA Redis usage 消息中已带错误码/原因字段；后端新增字段落库（实施时先抓一条失败消息确认实际 JSON 字段名） |
| 历史数据 | 不回填；旧失败事件该列为 `—` |
| 列默认位置 | `result` 之后 |
| 列自定义 | 加入 `REQUEST_EVENT_COLUMN_IDS`，自动出现在列设置里 |
| 归档表 | `usage_events_archive` 同步加列，保持结构一致 |
| 导出 | CSV/导出建议同步带上错误码和原因，保证数据一致 |

---

## 4. 后端设计

### 4.1 数据库迁移

新增迁移文件：

```text
internal/repository/migration/20260820_add_usage_event_error_fields.go
```

迁移名：`20260820_add_usage_event_error_fields`

逻辑（幂等）：

1. 若 `usage_events` 表存在且缺少 `error_code`，执行：

```sql
ALTER TABLE usage_events ADD COLUMN error_code TEXT;
```

2. 若 `usage_events` 表存在且缺少 `error_message`，执行：

```sql
ALTER TABLE usage_events ADD COLUMN error_message TEXT;
```

3. 若 `usage_events_archive` 表存在，同样补这两列。

在 `migration.go` 注册：

```go
migrationAddUsageEventErrorFields = "20260820_add_usage_event_error_fields"
```

并追加到 `orderedMigrations()` 末尾。

### 4.2 实体扩展

`internal/entities/usage_event.go` 的 `UsageEvent` 增加：

```go
ErrorCode    *string `gorm:"column:error_code"`
ErrorMessage *string `gorm:"column:error_message"`
```

`internal/entities/usage_event_archive.go` 的 `UsageEventArchive` 同步增加同名字段。

说明：

- 用 `*string` 区分“无错误信息”和空字符串；历史行和成功事件为 `NULL`。
- `InsertUsageEvents` 不需要改，GORM 自动写入新字段。

### 4.3 Redis 消息解码

`internal/service/redis_usage.go` 的 `queuedUsageDetail` 增加：

```go
ErrorCode    *string `json:"error_code"`
ErrorMessage *string `json:"error_message"`
```

实施时先抓一条失败消息确认字段名；如果 CPA 字段是 `error` / `status_code` / `message`，则用对应 json tag。

`toUsageEvent` 映射：

```go
event.ErrorCode = trimOptionalRedisString(d.ErrorCode)
event.ErrorMessage = trimOptionalRedisString(d.ErrorMessage)
```

- trim 后为空 → `nil`。
- 长度限制建议：`error_code` ≤ 64，`error_message` ≤ 1024（超长截断或置空，实施时确认）。

### 4.4 API 响应

`internal/api/usage_events.go`：

- `usageEventPayload` 增加：

```go
ErrorCode    *string `json:"error_code,omitempty"`
ErrorMessage *string `json:"error_message,omitempty"`
```

- 事件列表映射函数补上两个字段。
- `usageEventExportPayload` 建议也增加：

```go
ErrorCode    string `json:"error_code,omitempty"`
ErrorMessage string `json:"error_message,omitempty"`
```

- CSV 导出补两列（如现有导出结构允许）。

### 4.5 查询/筛选

本次不新增按错误码筛选；后续如需，可再加 `error_code` 筛选参数。

---

## 5. 前端设计

### 5.1 类型与 API

`web/src/lib/types.ts` 的 `UsageEvent` 增加：

```ts
error_code?: string | null
error_message?: string | null
```

API 客户端无需改动（字段自动透传）。

### 5.2 列定义

`web/src/components/usage/requestEventColumns.ts`：

```ts
export const REQUEST_EVENT_COLUMN_IDS = [
  'timestamp',
  'api_key',
  'source',
  'model',
  'model_alias',
  'reasoning_effort',
  'service_tier',
  'result',
  'error',       // 新增，默认在 result 后
  'request_type',
  'endpoint',
  ...
] as const;
```

`RequestEventColumnId` 自动包含 `'error'`。

### 5.3 列渲染

`web/src/components/usage/RequestEventsDetailsCard.tsx`：

- 在列配置数组中，`result` 后新增：

```tsx
{
  id: 'error',
  label: t('usage_stats.request_events_error'),
  header: <th className={styles.requestEventsNoWrapCell}>{t('usage_stats.request_events_error')}</th>,
  cell: (row) => (
    <span className={styles.requestEventsErrorCell} title={...}>
      {row.failed ? formatRequestEventError(row) : '—'}
    </span>
  ),
}
```

- 展示逻辑：

```ts
function formatRequestEventError(event: UsageEvent): string {
  if (!event.failed) return '—'
  const code = event.error_code?.trim()
  const message = event.error_message?.trim()
  if (code && message) return `${code} ${message}`
  if (code) return code
  if (message) return message
  return '—'
}
```

- 样式：`requestEventsErrorCell` 不换行，超长时省略并用 `title` 显示完整内容；错误列建议放在可横向滚动表格中，宽度自适应。

### 5.4 列设置与偏好迁移

- 列设置自动支持 `error` 列（因为走 `REQUEST_EVENT_COLUMN_IDS`）。
- `UsagePage.tsx` 用户偏好版本从 8 升到 9，并在恢复旧偏好时：
  - 若旧列顺序里没有 `error`，插入到 `result` 之后。
  - 旧可见性里没有 `error` 时，默认可见（与现有新增列逻辑一致）。

### 5.5 i18n

三语新增：

| key | en | zh | zh-TW |
|---|---|---|---|
| `request_events_error` | Error | 错误 | 錯誤 |

---

## 6. 测试计划

### 6.1 后端

| 文件 | 内容 |
|---|---|
| `internal/repository/migration/20260820_add_usage_event_error_fields_test.go` | 幂等加列；usage_events 与 archive 都加列 |
| `internal/service/redis_usage_test.go` | 失败消息解码 error_code/error_message；空值/缺字段/超长处理 |
| `internal/api/test/usage_events_test.go` | API 响应包含 error_code/error_message；成功事件为 null/omitempty |
| 归档测试 | 归档时新字段不丢失（如已有归档测试覆盖） |

### 6.2 前端

| 文件 | 内容 |
|---|---|
| `web/src/components/usage/test/RequestEventsDetailsCard.test.tsx` | `error` 列默认出现在 result 后；失败事件显示短码+原因；成功/历史失败显示 `—` |
| `web/src/components/usage/test/RequestEventsColumnSettings.test.tsx` | `error` 列可显示/隐藏/拖拽 |
| `web/src/pages/test/UsagePageRequestEventsPreferences.test.ts` | 旧偏好迁移：`error` 插入到 result 后且默认可见 |
| `web/src/i18n/index.test.ts` | 三语 key 存在 |
| `web/src/pages/test/UsagePage.styles.test.ts` | 回归 REQUEST_EVENT_COLUMN_IDS 相关断言 |

---

## 7. 新增/修改文件清单

### 7.1 后端新增

```text
internal/repository/migration/20260820_add_usage_event_error_fields.go
internal/repository/migration/20260820_add_usage_event_error_fields_test.go
```

### 7.2 后端修改

```text
internal/entities/usage_event.go
internal/entities/usage_event_archive.go
internal/repository/migration/migration.go
internal/service/redis_usage.go
internal/api/usage_events.go
```

### 7.3 前端修改

```text
web/src/lib/types.ts
web/src/components/usage/requestEventColumns.ts
web/src/components/usage/RequestEventsDetailsCard.tsx
web/src/components/usage/RequestEventsDetailsCard.module.scss（如需要）
web/src/pages/UsagePage.tsx（偏好版本迁移）
web/src/i18n/index.ts
```

---

## 8. 实施步骤建议

1. 后端迁移 + 实体字段。
2. Redis 解码新增字段（先抓一条失败消息确认 CPA 字段名）。
3. API 响应映射 + 单测。
4. 前端类型 + 列定义 + 列渲染 + i18n。
5. 用户偏好迁移（版本 8 → 9）。
6. 测试与回归。

---

## 9. 风险与后续

- **CPA 字段名不确认**：实施前必须先拿一条失败消息的原始 JSON 确认 `error_code` / `error_message` 的实际字段名；若没有这两个字段，需要回退到从 `response_headers` 解析。
- **历史数据为空**：旧失败事件该列显示 `—`，这是预期；如需回填历史，需要额外重放/解析方案，本次不做。
- **列宽**：错误原因可能很长，表格需保持横向滚动，并用 `title` 展示完整内容。
- **归档**：`usage_events_archive` 结构要保持一致，后续归档才能继续写新字段。
