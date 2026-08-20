# 方案：AI 供应商列表新增「总价格」列

## 1. 目标

在「AI 供应商」tab 的凭证列表中新增一列「总价格」，展示该 API Key（AI Provider 身份）从有数据以来的累计消耗总价格。

- 统计范围：**全部历史**（从该身份有数据开始到当前）。
- 价格口径：**按当前配置的价格计算历史 token**。不同 API Key/模型按各自当前配置的价格计算；配置了高峰/闲时的模型按高峰/闲时倍率计算；OpenAI 计价风格的按该风格配置价格计算。
- 列位置：**总 Token 之后、缓存命中率之前**。
- 货币单位：**USD（$）**。
- 失败请求是否计费：与现有费用统计保持一致（现有成本聚合不排除 failed 事件）。

本次只输出方案，不改代码。

---

## 2. 现状分析（已核对代码）

### 2.1 前端

- AI 供应商表格：`web/src/components/usage/credentials/AiProviderCredentialsSection.tsx`
  - 使用 `CredentialTableHeader` / `CredentialRowShell` 渲染表头与行。
  - 行内指标组当前为 4 列：请求数、成功率、总 Token、缓存命中率。
  - 右侧 side 区域为 `CredentialHealthPanel` + Session 配置按钮。
- 行数据来源：`useCredentialsTabData` → `useCredentialPages` → `fetchUsageIdentitiesPage({ authType: 2 })` → `buildAiProviderCredentialRows`。
- 表格列 CSS：`CredentialSections.module.scss`
  - `.credentialMetricHeaderGroup` 和 `.credentialMetricGroup` 当前都是 4 列 grid。
- 成本格式化参考：`credentialViewModels.ts` 已有 `formatQuotaWindowCost`（USD，2 位小数）。
- i18n：`web/src/i18n/index.ts` 中/英/繁三语。

### 2.2 后端

- AI Provider 身份：`usage_identities` 表，`auth_type = 2`，`identity` 为稳定 auth-index。
- 前端拿到的 AI Provider 身份 DTO 中 `identity` 是**脱敏后的 auth-index**，前端只有数字 `id` 可用作关联键。
- 费用计算：
  - 系统没有历史价格表，只有当前 `model_price_settings` 和 `model_price_rules`。
  - 成本在查询时实时计算，公式在 `internal/helper/usage_cost.go`：
    - 普通输入 = `input_tokens - cache_read_tokens - cache_creation_tokens`
    - `普通输入/1e6 * prompt_price + cache_read/1e6 * cache_read_price + cache_creation/1e6 * cache_write_price + output/1e6 * completion_price`
    - 再乘 `price_multiplier` 与 `pricing_period` 规则倍率。
  - Overview/Analysis 的成本数据源是 `usage_overview_hourly_stats` / `usage_overview_daily_stats`（按 `auth_index`、`model`、`pricing_period` 等维度聚合），不是逐条扫 `usage_events`。
  - `usage_overview_hourly_stats` / `usage_overview_daily_stats` 已有 `auth_index`、`model`、`model_alias`、`service_tier`、`response_service_tier`、`reasoning_effort`、`endpoint`、`executor_type`、`pricing_period` 以及各类 token 字段。
- 价格解析器：`internal/pricing` 包的 `Catalog` / `Resolver`，会根据模型、pricing_period 和规则自动给出倍率与最终单价。
- 装配：`internal/app/app.go` 中 `pricingCatalog` 在 `usageIdentityService` 之前创建，可注入。

---

## 3. 总体设计

### 3.1 数据流

```text
前端 AI 供应商 tab
   │  1. 已有：GET /usage/identities/page?auth_type=2  → 行数据
   │  2. 新增：GET /usage/identities/costs?auth_type=2 → 每个 identity_id 的总价
   ▼
后端：
   1. 读取 active 的 AI Provider identities（只取 id）
   2. 聚合 usage_overview_daily_stats + 今天 usage_overview_hourly_stats
      GROUP BY identity_id 对应的 auth_index + 价格维度
   3. 对每个聚合行用 pricing.Resolver 计算 cost
   4. 按 identity_id 汇总 total_cost_usd
   ▼
前端：
   把 total_cost_usd 合并进 aiProviderRows
   CredentialTableHeader / CredentialRowShell 增加第 5 个指标列
   列顺序：请求数 | 成功率 | 总 Token | 总价格 | 缓存命中率 | 健康状态
```

### 3.2 关键决策

| 决策点 | 结论 |
|---|---|
| 统计范围 | 全部历史 |
| 价格口径 | 用当前价格配置 + 当前高峰/闲时倍率重算历史 token |
| 数据源 | `usage_overview_daily_stats`（完整自然日）+ `usage_overview_hourly_stats`（今天），避免扫原始 `usage_events` |
| 关联键 | 后端返回 `identity_id`（数字 id），前端用 `row.identity.id` 合并 |
| 未配置价格的模型 | 该 identity 标记 `cost_available=false`，前端显示 `—` |
| 失败请求 | 与现有成本聚合一致，不额外排除 |
| 性能 | 一次聚合查询 + 内存计算，结果按 identity 缓存；前端只在 AI 供应商 tab 可见时请求，并带 5 分钟 TTL |

---

## 4. 后端设计

### 4.1 新接口

建议新增独立接口，避免改动现有分页接口的响应结构和签名：

```text
GET /api/v1/usage/identities/costs?auth_type=2
```

响应：

```json
{
  "items": [
    {
      "identity_id": "123",
      "total_cost_usd": 12.345678,
      "cost_available": true
    }
  ],
  "generated_at": "2026-08-20T10:00:00+08:00"
}
```

- `auth_type` 仅允许 `1` 或 `2`；AI 供应商页传 `2`。
- 后端自己读取 active identities，不信任前端传回的 auth_index（因为前端拿到的是脱敏值）。
- `identity_id` 使用 `usage_identities.id` 字符串。

### 4.2 聚合查询

在 `internal/repository` 新增：

```text
internal/repository/usage_identity_costs.go
```

核心函数：

```go
type UsageIdentityCostAggregate struct {
    AuthIndex           string
    APIGroupKey         string
    Model               string
    ModelAlias          string
    ServiceTier         string
    ResponseServiceTier string
    ReasoningEffort     string
    Endpoint            string
    ExecutorType        string
    PricingPeriod       string
    CostUncachedInputTokens int64
    CostOutputTokens    int64
    CostCacheReadTokens int64
    CostCacheCreationTokens int64
}

func AggregateUsageIdentityCosts(
    ctx context.Context,
    db *gorm.DB,
    authIndexes []string,
    now time.Time,
) ([]UsageIdentityCostAggregate, error)
```

实现要点：

1. **完整日**：查 `usage_overview_daily_stats`
   - 条件：`auth_index IN ? AND bucket_start < 今天本地日 00:00`
   - 分组：`auth_index, api_group_key, model, model_alias, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, pricing_period`
   - 聚合列复用 `usageOverviewStatProjectionAggregateColumns` 的 SQL CASE 口径，保证与 Overview 成本完全一致。

2. **今天**：查 `usage_overview_hourly_stats`
   - 条件：`auth_index IN ? AND bucket_start >= 今天本地日 00:00`
   - 分组与聚合列同上。

3. 两次查询结果合并，交给 service 计算成本。

说明：

- 如果只查 `usage_overview_daily_stats`，今天的数据会缺失；因此必须补今天的小时表。
- 如果后续有归档策略，`usage_events_archive` 也可作为兜底；本方案先以 daily + 今天 hourly 为准。

### 4.3 成本计算

在 `internal/service` 新增：

```text
internal/service/usage_identity_cost_service.go
```

```go
type UsageIdentityCostProvider interface {
    ListUsageIdentityCosts(ctx context.Context, authType entities.UsageIdentityAuthType) (UsageIdentityCostsResponse, error)
}
```

实现流程：

1. `repository.ListActiveUsageIdentities(ctx, db)` 过滤出目标 `auth_type` 的 identities，得到 `id → auth_index` 映射。
2. `repository.AggregateUsageIdentityCosts(ctx, db, authIndexes, now)` 获取聚合行。
3. 对每个聚合行：
   - 构造 `pricing.NewCostSubject` / `UsagePricingCostSubject`。
   - 调 `pricing.Resolver.Calculate` 得到 `TotalCostUSD`。
   - 按 `auth_index` 累加。
4. 将 `auth_index` 映射回 `identity_id`。
5. 返回结果；`cost_available=false` 的 identity 仍返回条目，`total_cost_usd=0`。

依赖：

- service 需要 `*gorm.DB` 和 `pricing.Catalog`（或 `pricing.Resolver`）。
- 在 `app.go` 装配时注入 `pricingCatalog`。

### 4.4 路由与装配

- `internal/api/usage_identities.go` 或新文件 `internal/api/usage_identity_costs.go` 注册：

```go
router.GET("/usage/identities/costs", func(c *gin.Context) {
    // 解析 auth_type，只允许 1/2
    // 调用 UsageIdentityCostProvider
})
```

- `internal/api/router.go` 的 `OptionalProviders` 增加 `UsageIdentityCosts service.UsageIdentityCostProvider`。
- `internal/app/app.go`：
  - 创建 `usageIdentityCostService := service.NewUsageIdentityCostService(db, pricingCatalog)`。
  - 注入 `OptionalProviders`。

### 4.5 错误与边界

- `auth_type` 非法 → 400。
- DB 查询失败 → 500。
- 某模型未配置价格 → 该聚合行不产生 cost，identity 仍返回 `cost_available=false`。
- 空数据 → `items` 为空数组，不报错。
- 时间边界使用项目本地时区（与现有 `timeutil` 口径一致）。

---

## 5. 前端设计

### 5.1 类型与 API

`web/src/lib/types.ts`：

```ts
export interface UsageIdentityCostItem {
  identity_id: string
  total_cost_usd: number
  cost_available: boolean
}

export interface UsageIdentityCostsResponse {
  items: UsageIdentityCostItem[]
  generated_at: string
}
```

`web/src/lib/api.ts`：

```ts
export async function fetchUsageIdentityCosts(
  authType: UsageIdentityAuthType,
  signal?: AbortSignal,
): Promise<UsageIdentityCostsResponse> {
  // GET /usage/identities/costs?auth_type=2
}
```

### 5.2 数据层

`web/src/components/usage/credentials/useCredentialPages.ts`：

- 不直接改分页接口；新增独立 hook 或状态：

```ts
const [aiProviderCosts, setAiProviderCosts] = useState<Map<string, UsageIdentityCostItem>>(new Map())
```

- 在 AI Provider 页可见时，调用 `fetchUsageIdentityCosts(2)`，结果按 `identity_id` 存 Map。
- 5 分钟 TTL；身份列表刷新时不清空旧成本（避免列抖动）。

`web/src/components/usage/credentials/useCredentialsTabData.ts`：

- `CredentialsTabData` 增加：

```ts
aiProviderCosts: Map<string, UsageIdentityCostItem>
refreshUsageIdentityCosts: () => Promise<void>
```

- `refresh` 时同时刷新身份和成本（成本走缓存 TTL）。

`web/src/components/usage/credentials/credentialViewModels.ts`：

- `AiProviderCredentialRow` 增加：

```ts
totalCostUsd?: number
costAvailable?: boolean
```

- `buildAiProviderCredentialRows` 增加可选参数 `costs?: Map<string, UsageIdentityCostItem>`，把成本合并进每一行。

### 5.3 表头与行渲染

`web/src/components/usage/credentials/CredentialSectionShell.tsx`：

- `CredentialTableHeader` 增加可选 props：

```ts
totalCostLabel?: string
showTotalCost?: boolean
```

- `CredentialRowShell` 增加可选 prop：

```ts
totalCost?: ReactNode
```

- 仅 AI Provider 传 `totalCostLabel` / `totalCost`；Auth Files 不传，保持 4 列。

`web/src/components/usage/credentials/AiProviderCredentialsSection.tsx`：

- `CredentialTableHeader` 传：

```tsx
totalCostLabel={t('usage_stats.credentials_column_total_cost')}
showTotalCost
```

- 每行 `metrics` 在总 Token 后插入：

```tsx
<MetricPill value={formatUsageIdentityTotalCost(row.totalCostUsd, row.costAvailable)} />
```

- 新格式化函数（建议放 `credentialViewModels.ts` 或 `usage.ts`）：

```ts
export function formatUsageIdentityTotalCost(
  costUsd: number | undefined,
  available: boolean | undefined,
): string {
  if (!available || costUsd === undefined) return '—'
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(costUsd)
}
```

- 如果 `0 < costUsd < 0.005`，显示 `<$0.01`，避免显示成 `$0.00` 引起误解。

### 5.4 样式

`web/src/components/usage/credentials/CredentialSections.module.scss`：

- 当前 4 列 grid：

```scss
.credentialMetricHeaderGroup,
.credentialMetricGroup {
  grid-template-columns:
    minmax(136px, 1.45fr)
    minmax(88px, 0.9fr)
    minmax(92px, 0.9fr)
    minmax(84px, 0.8fr);
}
```

- 新增 AI Provider 专用 5 列修饰类：

```scss
.credentialMetricGroupAiProvider {
  grid-template-columns:
    minmax(136px, 1.45fr)
    minmax(88px, 0.9fr)
    minmax(92px, 0.9fr)
    minmax(110px, 1fr)
    minmax(84px, 0.8fr);
}
```

- `.aiProviderCredentialRow` 使用 5 列；Auth Files 保持原 4 列。
- 移动端继续沿用 2 列换行逻辑。

### 5.5 i18n

三语新增 `usage_stats.credentials_column_total_cost`：

| key | en | zh | zh-TW |
|---|---|---|---|
| `credentials_column_total_cost` | Total Cost | 总价格 | 總價格 |

---

## 6. 测试计划

### 6.1 后端

| 文件 | 内容 |
|---|---|
| `internal/repository/usage_identity_costs_test.go` | 用临时 DB seed daily/hourly stats，验证按 auth_index 聚合、今天边界、token CASE 口径 |
| `internal/service/usage_identity_cost_service_test.go` | 用 seed 价格规则验证 peak/off-peak 倍率、OpenAI 风格价格、未配置价格时 cost_available=false |
| `internal/api/usage_identity_costs_test.go` | 路由鉴权、auth_type 参数校验、返回 JSON 结构、空数据 |
| 回归 | 确认现有 Overview/Analysis 成本测试不受影响 |

### 6.2 前端

| 文件 | 内容 |
|---|---|
| `web/src/lib/test/api.test.ts` | 新 API 路径、query 参数、错误处理 |
| `web/src/components/usage/credentials/test/credentialViewModels.test.ts` | `buildAiProviderCredentialRows` 合并成本、缺省显示 |
| `web/src/components/usage/credentials/AiProviderCredentialsSection.test.tsx` | AI Provider 表格出现「总价格」表头与列值；Auth Files 不出现 |
| `web/src/components/usage/credentials/test/CredentialSections.styles.test.ts` | 5 列 grid 样式只作用于 AI Provider 行 |
| `web/src/i18n/index.test.ts` | 新增三语 key |
| `web/src/pages/test/UsagePage.styles.test.ts` | 回归：AI 供应商页仍通过现有 toolbar/布局断言 |

---

## 7. 新增/修改文件清单

### 7.1 后端新增

```text
internal/repository/usage_identity_costs.go
internal/repository/usage_identity_costs_test.go
internal/service/usage_identity_cost_service.go
internal/service/usage_identity_cost_service_test.go
internal/api/usage_identity_costs.go（或并入 usage_identities.go）
internal/api/usage_identity_costs_test.go
```

### 7.2 后端修改

```text
internal/api/router.go
internal/app/app.go
```

### 7.3 前端新增

```text
无新文件（除非把 cost 合并逻辑抽成独立 hook：web/src/components/usage/credentials/useUsageIdentityCosts.ts）
```

### 7.4 前端修改

```text
web/src/lib/types.ts
web/src/lib/api.ts
web/src/i18n/index.ts
web/src/components/usage/credentials/useCredentialPages.ts
web/src/components/usage/credentials/useCredentialsTabData.ts
web/src/components/usage/credentials/credentialViewModels.ts
web/src/components/usage/credentials/AiProviderCredentialsSection.tsx
web/src/components/usage/credentials/CredentialSectionShell.tsx
web/src/components/usage/credentials/CredentialSections.module.scss
```

---

## 8. 实施步骤建议

1. 后端 repository 聚合函数 + 单测。
2. 后端 service 成本计算 + 单测。
3. 后端 API 路由 + 装配 + 单测。
4. 前端类型、API client、i18n。
5. 数据层合并成本。
6. 表头/行 UI 与 5 列样式。
7. 前端测试与回归。
8. 手工验证：AI 供应商 tab 中 Total Cost 列显示、Auth Files 不变、暗色/移动端布局。

---

## 9. 风险与后续

- **性能**：所有历史 daily + 今天 hourly 聚合按 auth_index 分组，数据量可控；若身份数很多，可在 repository 层对 auth_index 分页/分批查询。
- **价格变更**：系统无历史价格表，历史费用按当前价格重算；后续如需精确历史账单，需要新增价格快照表，本次不实现。
- **缓存一致性**：成本缓存 5 分钟；用户刚改价格后需要等缓存或手动刷新身份/成本。
- **多模型/多规则**：一个 API Key 可能调用多个模型，按每个聚合行的实际维度分别计价后汇总。
