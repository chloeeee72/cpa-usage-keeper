# 方案：AI 供应商页余额查询（TokenRhythm Session 余额）

## 1. 目标

1. 在「AI 供应商」tab 的刷新按钮**左侧**新增「余额查询」按钮；按钮仅在 `activeTab === 'ai-provider'` 时显示。
2. 点击按钮打开余额弹窗，展示：
   - 顶部**总计模块**：总余额、可用、冻结、限时、累计调用、累计成本。
   - 每个配置了 session 的 API Key：**数据表 + 柱状图 + 环形占比图**。
   - session 为空或未配置的 API Key **不在弹窗中展示**。
3. 每个 API 可手动配置 `tr_session` Cookie 值并写入数据库：
   - 仅 **TokenRhythm（tokenrhythm.studio）** 供应商的 AI Provider 行允许配置 session。
   - 其他供应商行不显示 session 配置入口，保持现状。
   - 配置入口放在 AI 供应商列表**每行的行内按钮**上，点击打开小弹窗填写/清空。
4. 余额数据源固定为 `https://tokenrhythm.studio/api/usage-summary`，后端以 `Cookie: tr_session=<session>` 请求，解析 `data` 字段。
5. 打开弹窗时**自动查询**所有已配置 session 的 API Key 余额。
6. 弹窗内提供**刷新按钮**，点击重新获取全部 API Key 余额数据并更新弹窗内容。
7. 每个 API Key 行提供独立的**「余额查询」按钮**，只更新该 API Key 的余额数据，不影响其他行。
8. UI 风格与现有项目一致（复用 `Modal` / `Button` / `Card` / SCSS 变量 / i18n / Chart.js）。

本次只输出方案，不改代码。

---

## 2. 现状分析（已核对代码）

### 2.1 前端

- 页面主文件：`web/src/pages/UsagePage.tsx`
  - `USAGE_TAB_OPTIONS = ['overview', 'analysis', 'events', 'auth-files', 'ai-provider', 'settings']`
  - 刷新按钮位于 `toolbarActionsRight` → `usageRefreshSlot` → `MainActionButton`（约 L2029–L2047），是工具栏右侧唯一的操作按钮。
  - AI 供应商列表渲染：`credentialSectionVisibility.showAiProvider && <AiProviderCredentialsSection rows={credentialsData.aiProviderRows} ... />`（L2188–L2203）。
- AI 供应商行数据来自 `useCredentialsTabData` → `useCredentialPages` → `fetchUsageIdentitiesPage({ authType: 2, ... })`，行模型由 `buildAiProviderCredentialRows` 生成。
- 每行已有：品牌图标、别名编辑（`CredentialAliasEditor`）、优先级徽标、请求/成功率/Token/缓存命中率指标、健康面板（`CredentialHealthPanel`）。
- 已有可复用组件：
  - `web/src/components/ui/Modal.tsx`（焦点管理、滚动锁、关闭动画）。
  - `web/src/components/ui/Button.tsx`（`variant: primary|secondary|ghost|danger`）。
  - `web/src/components/ui/Input.tsx`、`Card.tsx`。
  - 图表：`chart.js@4` + `react-chartjs-2@5`，统一注册在 `web/src/lib/chartjs.ts`；`AnalysisPanel.tsx` 中有成熟的 `Bar` / `Doughnut` 用法与暗色主题配色可参考。
  - i18n：`web/src/i18n/index.ts` 内 `usage_stats.*`，中/英/繁三语都在同一文件。

### 2.2 后端

- 框架：Go + Gin + GORM（SQLite，带 dbresolver 读写分离）。
- AI Provider 身份实体：`internal/entities/usage_identity.go`
  - `auth_type = 2` 表示 AI Provider；`identity` 为稳定 auth-index。
  - 已有 `LookupKey`（CPA api-key）、`BaseURL`（provider base url）、`Alias`、`AccountID` 等字段。
  - `BaseURL` 来自 CPA 元数据同步（`metadata_provider.go`），OpenAI 兼容供应商（如 OpenRouter）会把 provider 级 `base_url` 写入该字段。
- 身份列表 API：`internal/api/usage_identities.go`
  - `GET /usage/identities`、`GET /usage/identities/page`、`PATCH /usage/identities/:id`（目前仅 alias）。
  - `mapUsageIdentityResponseWithHealth` 是统一 DTO 映射点；**故意不发布 `base_url` / `lookup_key`**（已有测试保护）。
- 仓储：`internal/repository/usage_identities.go` 有 `ListActiveUsageIdentitiesPage`、`UpdateUsageIdentityAlias`、`FindUsageIdentityByID`、`ReplaceUsageIdentitiesForProviderTypes` 等。
- 迁移机制：`internal/repository/migration/migration.go`
  - 按日期命名 `20260818_xxx`，函数幂等（`HasTable` / `HasColumn` 判断），在 `orderedMigrations()` 注册。
- 外部 HTTP 调用参考：
  - `internal/quota/*` 通过 CPA 管理 API 代理调用官方限额接口。
  - 本项目还没有直连外部站的 HTTP client，余额查询需要新增一个**直连 tokenrhythm.studio** 的小 client（自建 `http.Client`，不复用 CPA 客户端）。
- 路由与装配：
  - `internal/api/router.go` 的 `OptionalProviders` 注入各服务，`adminProtected` 下注册业务路由。
  - `internal/app/app.go` 创建各 service 并传入 `api.NewRouter(...)`。

---

## 3. 总体设计

### 3.1 数据流

```text
AI 供应商行（仅 tokenrhythm.studio 行显示 Session 按钮）
   │  点击 Session 按钮 → BalanceSessionModal
   │  PATCH /api/v1/usage/identities/:id/balance-session  {session}
   ▼
usage_identities.balance_session  （TEXT NULL，空串/ NULL = 未配置）

点击「余额查询」按钮 → BalanceQueryModal（打开时自动请求全部）
   │  POST /api/v1/usage/balance/query            （不带 identity_id）
   ▼
后端 balance service：
   1. 查询所有 active 的 AI Provider 身份
   2. 过滤：BaseURL 主机为 tokenrhythm.studio 且 balance_session 非空
   3. 并发（限流 4）请求 https://tokenrhythm.studio/api/usage-summary
      Header: Cookie: tr_session=<session>
   4. 汇总成功项，失败项逐条返回 error，不阻断其他账号
   ▼
弹窗展示：总计模块 + 柱状图 + 环形图 + 明细表

弹窗内「刷新」按钮
   │  POST /api/v1/usage/balance/query（不带 identity_id，重新查询全部）
   ▼
   前端保留旧数据展示，请求完成后整体替换为最新数据

明细表每行「余额查询」按钮
   │  POST /api/v1/usage/balance/query   body: {"identity_id": "<id>"}
   ▼
   后端只查询该 identity_id（校验 TokenRhythm 且 session 非空）
   ▼
   前端用返回的单行结果替换该行，并本地重算总计/图表
```

### 3.2 关键决策

| 决策点 | 结论 |
|---|---|
| 查询是否走后端 | **必须走后端**：避免浏览器跨域与 Cookie 隐私问题，session 不出浏览器也可复用 |
| 查询方式 | **同步请求 + 服务端并发**：账号数少、接口快；设置整体超时 10s，单账号 8s |
| 全量 vs 单行 | 同一 `POST /usage/balance/query`：不带 `identity_id` 查全部；带 `identity_id` 只查该行。弹窗打开和「刷新」按钮走全量，行内「余额查询」按钮走单行 |
| session 存储 | `usage_identities.balance_session TEXT NULL`，与 `LookupKey` 同属本地敏感字段 |
| session 是否返回前端 | **不返回明文**；列表 DTO 只返回 `balance_session_supported` / `balance_session_configured` 两个布尔值 |
| 哪些行可配 session | 仅后端判定 `BaseURL` 主机为 `tokenrhythm.studio` 的行（兼容大小写与端口） |
| 其他供应商 | 不显示配置入口，余额查询自动忽略 |

---

## 4. 后端设计

### 4.1 数据库迁移

新增迁移文件：

```text
internal/repository/migration/20260819_add_usage_identity_balance_session.go
```

迁移名：`20260819_add_usage_identity_balance_session`

逻辑（幂等）：

```go
func addUsageIdentityBalanceSessionMigration(tx *gorm.DB) error {
    if !tx.Migrator().HasTable(&entities.UsageIdentity{}) ||
       tx.Migrator().HasColumn(&entities.UsageIdentity{}, "balance_session") {
        return nil
    }
    return tx.Migrator().AddColumn(&entities.UsageIdentity{}, "BalanceSession")
}
```

在 `migration.go` 中：

- 新增常量 `migrationAddUsageIdentityBalanceSession = "20260819_add_usage_identity_balance_session"`。
- 在 `orderedMigrations()` 末尾追加：

```go
{version: migrationAddUsageIdentityBalanceSession, run: addUsageIdentityBalanceSessionMigration},
```

### 4.2 实体扩展

`internal/entities/usage_identity.go` 的 `UsageIdentity` 增加：

```go
BalanceSession *string
```

说明：

- 与 `Alias` / `Note` / `AccountID` 一样使用指针，`NULL` 表示未配置，`""` 视为未配置。
- 不在任何查询投影中遗漏：`internal/repository/usage_identities.go` 的 `usageIdentityReadColumns` 需追加 `balance_session`。
- `ReplaceUsageIdentitiesForProviderTypes` / `ReplaceUsageIdentitiesForAuthType` 的 upsert 字段需要确认是否需要保留该字段（**不能**在 provider 元数据同步时覆盖手动配置的 session）。建议这两个 scoped replace 的更新列**不包含** `balance_session`，避免 CPA 同步冲掉用户配置；同理 `merge` 逻辑中保留存量 session。

### 4.3 Repository 扩展

`internal/repository/usage_identities.go` 增加：

```go
func UpdateUsageIdentityBalanceSession(ctx context.Context, db *gorm.DB,
    id int64, session *string) error
```

实现要点：

- `id <= 0` 返回 `gorm.ErrRecordNotFound`（或沿用 alias 的 `ErrInvalidID` 分层）。
- 只更新 `balance_session` 字段（空串写入 `NULL`，null 写入 `NULL`）。
- 更新前检查行存在且 `is_deleted = false`，保证只允许活跃身份。
- 通过 dbresolver 自动路由 writer。

### 4.4 TokenRhythm 余额 client

新增包 `internal/balance/`（建议独立小包，不污染 `quota`）：

```text
internal/balance/client.go
internal/balance/types.go
```

`types.go` 定义（字段名与 demo 对齐，使用 Go JSON tag 原样解析）：

```go
type UsageSummary struct {
    BalanceCny           float64 `json:"balanceCny"`
    AvailableBalanceCny  float64 `json:"availableBalanceCny"`
    FrozenBalanceCny     float64 `json:"frozenBalanceCny"`
    ExpiringBalanceCny   float64 `json:"expiringBalanceCny"`
    NextExpiryAt         string  `json:"nextExpiryAt"`
    Calls                int64   `json:"calls"`
    CostCny              float64 `json:"costCny"`
}
```

`client.go`：

```go
type Client struct {
    BaseURL    string        // 默认 https://tokenrhythm.studio
    HTTPClient *http.Client  // Timeout 8s，不跳过 TLS 校验
}

func (c *Client) QueryUsageSummary(ctx context.Context, session string) (UsageSummary, error)
```

要点：

- `GET <BaseURL>/api/usage-summary`。
- Header：`Cookie: tr_session=<session>`、`Accept: application/json`。
- `401` 返回专用错误 `ErrUnauthorized`（前端展示「session 失效或格式错误」）。
- 非 2xx：解析 body 中的 `error` / `message`，返回带状态码的错误。
- 成功：解析 `{"data": {...}}` 到 `UsageSummary`。
- **日志脱敏**：错误信息与日志中不得出现完整 session（只记录前 4 后 4 或直接不记录）。

### 4.5 Balance service

新增 `internal/service/balance_service.go`：

```go
type BalanceProvider interface {
    QueryBalances(ctx context.Context, identityID *int64) (BalanceQueryResponse, error)
    UpdateUsageIdentityBalanceSession(ctx context.Context, id int64, session string) (entities.UsageIdentity, error)
}
```

`UpdateUsageIdentityBalanceSession`：

1. `id <= 0` → `ErrInvalidID`。
2. `session = strings.TrimSpace(session)`。
3. 校验：
   - 长度 ≤ 4096。
   - 禁止控制字符（`unicode.IsControl`，允许 `\t` 视情况，建议禁止所有控制字符与零宽/双向控制符，复用 alias 的格式校验思路）。
4. 读取身份，校验：
   - 存在且未删除。
   - `AuthType == AIProvider`。
   - `SupportsTokenRhythmBalance(identity)` 为 true，否则返回 `ErrUnsupportedType`。
5. 调 repository 写库；空串清空为 `NULL`。
6. 返回更新后的 identity（经 DTO 映射后含 `balance_session_configured`）。

`QueryBalances(ctx, identityID *int64)`：

- **单行查询**（`identityID != nil`）：
  1. `repository.FindUsageIdentityByID(ctx, db, *identityID)` 读取身份。
  2. 校验存在、未删除、`AuthType == AIProvider`、`SupportsTokenRhythmBalance` 为 true、`BalanceSession` 非空；任一不满足返回 `ErrNotFound` / `ErrUnsupportedType` / `ErrValidation`（由 API 层映射为 404 / 400）。
  3. 只请求该行 session，返回 `BalanceQueryResponse{ Items: [该行], Totals: 该行汇总, ... }`。
- **全量查询**（`identityID == nil`）：
  1. `repository.ListActiveUsageIdentities(ctx, db)`（或新增专用过滤查询，避免一次取全量；当前 active 量小，先复用）。
  2. 过滤：

```go
func SupportsTokenRhythmBalance(item entities.UsageIdentity) bool {
    host := hostFromURL(item.BaseURL)   // 忽略大小写、端口
    return host == "tokenrhythm.studio" ||
           strings.HasSuffix(host, ".tokenrhythm.studio")
}
```

   （如果 CPA 中 TokenRhythm 未配 `base_url` 而只配了名称，可增加 `Provider`/`Name` 包含 `tokenrhythm` 的宽松判断；方案默认以 `BaseURL` 为准，落地时按实际 CPA 配置确认。）

  3. 过滤 `BalanceSession == nil || *BalanceSession == ""` 的行——这些行**不参与查询、不出现在返回结果中**。
  4. 并发请求（`errgroup` 或 channel 限流 4）：

```text
成功项 → items[] BalanceItem {identity_id, display_name, type, provider, summary}
失败项 → items[] BalanceItem {identity_id, display_name, type, provider, error}
```

  5. 汇总：

```go
type BalanceQueryResponse struct {
    Items           []BalanceQueryItem `json:"items"`
    Totals          BalanceTotals      `json:"totals"`
    ConfiguredCount int                `json:"configured_count"` // 支持且已配 session 的行数
    SucceededCount  int                `json:"succeeded_count"`
    FailedCount     int                `json:"failed_count"`
    GeneratedAt     time.Time          `json:"generated_at"`
}
```

- `Totals` 只累加成功项，包含：`balance_cny`、`available_balance_cny`、`frozen_balance_cny`、`expiring_balance_cny`、`calls`、`cost_cny`。
- 全量查询且无任何已配置 session 时返回空 items + 零值 totals，不报错（前端展示空态）。
- 单行查询失败时直接返回错误（由该行的独立「余额查询」按钮展示为行内错误），不返回 200 + error item。

### 4.6 API 路由

#### 4.6.1 identity DTO 扩展

`internal/api/usage_identities.go` 的 `usageIdentityResponse` 增加：

```go
BalanceSessionSupported   bool `json:"balance_session_supported"`
BalanceSessionConfigured  bool `json:"balance_session_configured"`
```

在 `mapUsageIdentityResponseWithHealth` 中填充：

```go
BalanceSessionSupported:  service.SupportsTokenRhythmBalance(item),
BalanceSessionConfigured: item.BalanceSession != nil && strings.TrimSpace(*item.BalanceSession) != "",
```

- 注意：**不发布** `balance_session` 明文，也不发布 `base_url`。
- `SupportsTokenRhythmBalance` 建议放在 service 包导出，api 层复用；若放 `internal/balance` 包也可，保持单向依赖。

#### 4.6.2 新路由

在 `internal/api/router.go`：

- `OptionalProviders` 增加 `Balance service.BalanceProvider`（字段名如 `Balance`）。
- 在 `adminProtected` 下注册：

```go
registerBalanceRoutes(adminProtected, balanceProvider)
```

新增 `internal/api/balance.go`：

```text
PATCH /usage/identities/:id/balance-session
body: { "session": "xxx" } | { "session": null }   // null/空串 = 清空
response: usageIdentityResponse

POST /usage/balance/query
body: 可选 { "identity_id": "<数字 id>" } | 空 body
  - 不传 identity_id → 查询全部已配置 session 的 TokenRhythm 行
  - 传 identity_id   → 只查询该行
response: BalanceQueryResponse（单行查询时 items 只含该行，totals 为该行汇总）
```

错误映射：

- `ErrInvalidID` → 400（identity_id 非法或不可解析）。
- `ErrUnsupportedType` → 400（该身份不是 TokenRhythm AI Provider）。
- `ErrNotFound` / `gorm.ErrRecordNotFound` → 404（身份不存在/已删除/未配置 session）。
- 上游 `balance.ErrUnauthorized` → 502（与 quota 的处理保持一致：不把外部 401 误认为登录态失效）；其他上游错误按状态码透传（4xx/5xx）。
- session 非法字符 → 400。

`POST /usage/balance/query` 全程使用当前登录态（admin），与 quota 路由一致。

### 4.7 装配

`internal/app/app.go`：

1. 创建 balance client：

```go
balanceClient := balance.NewClient(balance.ClientOptions{
    BaseURL:    "https://tokenrhythm.studio",
    HTTPClient: &http.Client{Timeout: 8 * time.Second},
})
```

2. 创建 service：

```go
balanceService := service.NewBalanceService(db, balanceClient)
```

3. 传入路由：

```go
api.OptionalProviders{
    ...
    Balance: balanceService,
}
```

说明：不复用 `cfg.TLSSkipVerify`（该配置只针对 CPA 与 Redis 连接；对公共站点保持 TLS 校验开启）。

### 4.8 安全与边界

- session 属于敏感凭证：不进入任何 GET 响应、不打印日志、错误信息中不包含 session 原文。
- PATCH 请求体不设大小写转换，只 trim；存储为 `*string`。
- 仅 AI Provider + TokenRhythm BaseURL 可写 session；Auth File 永远不可写。
- 后端查询只读取当前 active 且未删除的身份。
- 并发请求设置上限（4），避免账号多时打爆本地网络或对端。
- 上游 `usage-summary` 若返回 401，单条失败但整体 200 返回，弹窗内标红该行。

---

## 5. 前端设计

### 5.1 类型定义

`web/src/lib/types.ts`：

- `UsageIdentity` 增加：

```ts
balance_session_supported?: boolean
balance_session_configured?: boolean
```

- 新增：

```ts
export interface TokenRhythmBalanceSummary {
  balanceCny: number
  availableBalanceCny: number
  frozenBalanceCny: number
  expiringBalanceCny: number
  nextExpiryAt: string
  calls: number
  costCny: number
}

export interface TokenRhythmBalanceTotals {
  balance_cny: number
  available_balance_cny: number
  frozen_balance_cny: number
  expiring_balance_cny: number
  calls: number
  cost_cny: number
}

export interface BalanceQueryItem {
  identity_id: string
  display_name: string
  type: string
  provider: string
  error?: string
  summary?: TokenRhythmBalanceSummary
}

export interface BalanceQueryResponse {
  items: BalanceQueryItem[]
  totals: TokenRhythmBalanceTotals
  configured_count: number
  succeeded_count: number
  failed_count: number
  generated_at: string
}
```

### 5.2 API 客户端

`web/src/lib/api.ts` 新增：

```ts
export async function fetchBalanceSummary(options?: {
  identityId?: string
  signal?: AbortSignal
}): Promise<BalanceQueryResponse> {
  // POST /usage/balance/query
  // options.identityId 存在时 body = { identity_id: identityId }，否则 body = {}
}

export async function updateUsageIdentityBalanceSession(
  id: string, session: string | null,
): Promise<UsageIdentity> {
  // PATCH /usage/identities/:id/balance-session
}
```

- 两者都走 `apiFetch`（自动带登录 cookie / embed header）。
- 401 时抛 `ApiError(..., 401)`，调用方统一触发 `onAuthRequired`。

### 5.3 数据层

`web/src/components/usage/credentials/useCredentialPages.ts`：

- `mergeUsageIdentityAliasUpdate` 泛化为 `mergeUsageIdentityUpdate`，合并字段增加：

```ts
balance_session_supported: updated.balance_session_supported ?? current.balance_session_supported,
balance_session_configured: updated.balance_session_configured ?? current.balance_session_configured,
```

- `replaceUsageIdentity` 使用新的 merge 函数（改名后同步更新引用）。

`web/src/components/usage/credentials/useCredentialsTabData.ts`：

- `CredentialsTabData` 增加：

```ts
balanceSessionSavingId: string
saveUsageIdentityBalanceSession: (id: string, session: string | null) => Promise<void>
```

- 实现与 `saveUsageIdentityAlias` 对称：
  - `setBalanceSessionSavingId(id)`。
  - 调 `updateUsageIdentityBalanceSession`。
  - 成功后 `credentialPages.replaceUsageIdentity(updated)`，`onNotice('success', i18n.t('usage_stats.credentials_balance_session_save_success'))`。
  - 401 → `onAuthRequired()`；其他错误 → `onNotice('error', ...)` 并 rethrow。

`UsagePage.tsx`：

- `useCredentialsTabData` 的返回值透传给 `AiProviderCredentialsSection`（新增 props）。

### 5.4 工具栏按钮

`web/src/pages/UsagePage.tsx`：

- 新增 state：`const [balanceModalOpen, setBalanceModalOpen] = useState(false)`。
- 在 `styles.usageRefreshSlot` **之前**插入：

```tsx
{activeTab === 'ai-provider' && (
  <div className={styles.balanceQuerySlot}>
    <Button
      type="button"
      variant="secondary"
      className={styles.balanceQueryButton}
      onClick={() => setBalanceModalOpen(true)}
    >
      <IconDollarSign size={14} />
      <span>{t('usage_stats.balance_query')}</span>
    </Button>
  </div>
)}
```

- `usageRefreshSlot` 保留原样；按钮只影响当前行。
- 页面末尾渲染：

```tsx
<BalanceQueryModal
  open={balanceModalOpen}
  onClose={() => setBalanceModalOpen(false)}
  onAuthRequired={onAuthRequired}
/>
```

`web/src/pages/UsagePage.module.scss`：

- 新增 `.balanceQuerySlot` 与 `.balanceQueryButton`，与 `.usageRefreshSlot` / `.refreshMainActionShell` 对齐（同高度、间距、响应式规则与现有媒体查询保持一致）。

### 5.5 Session 配置入口（行内）

`web/src/components/usage/credentials/AiProviderCredentialsSection.tsx`：

- props 增加：

```ts
balanceSessionSavingId?: string
onSaveBalanceSession?: (id: string, session: string | null) => Promise<void>
```

- 行渲染处：仅当 `row.identity.balance_session_supported === true` 时，在 `side`（健康面板旁）或 `subtitle` 徽章区增加一个小按钮：

```tsx
<Button variant="ghost" size="sm" onClick={() => setEditingRow(row)}>
  <IconKey size={12} />
  {row.identity.balance_session_configured
    ? t('usage_stats.credentials_balance_session_configured')
    : t('usage_stats.credentials_balance_session_configure')}
</Button>
```

- 点击后打开 `BalanceSessionModal`，传入 `identityId`、`displayName`、当前是否已配置（不含明文）。

新组件 `web/src/components/usage/credentials/BalanceSessionModal.tsx`：

- 复用 `Modal`（`width={440}`）。
- 表单：`Input`（`type="password"`，带 `IconEye` / `IconEyeOff` 明文切换）。
- 说明文案：`tr_session` Cookie 值，从浏览器 DevTools 获取。
- 操作：保存（调 `onSaveBalanceSession(id, value)`，保存成功后关闭）、清空（调 `onSaveBalanceSession(id, null)`）。
- 校验：非空时 trim 后长度 ≥ 8（仅前端提示），不能包含换行/控制字符（简单正则）。

### 5.6 余额查询弹窗

新目录：

```text
web/src/components/usage/balance/
  BalanceQueryModal.tsx
  BalanceQueryModal.module.scss
  BalanceSummaryCards.tsx      // 总计模块
  BalanceCharts.tsx            // 柱状图 + 环形图
  BalanceItemsTable.tsx        // 明细表
  balanceChartConfig.ts        // chart data/options（可测试）
```

`BalanceQueryModal.tsx`：

- props：`open`、`onClose`、`onAuthRequired`。
- state：
  - `data: BalanceQueryResponse | null`
  - `loading`：首次全量加载（显示 `LoadingSpinner`）
  - `refreshing`：弹窗内点「刷新」时的全量刷新（保留旧数据展示，只禁用刷新按钮 + 显示刷新中）
  - `refreshingIdentityId: string | null`：正在单行刷新的行 id
- 打开时自动请求 `fetchBalanceSummary()`（带 AbortController）；关闭时中断未完成请求。
- 401 → `onAuthRequired()`。
- 空态：`configured_count === 0` 时显示「还没有配置余额查询 session」，并提供引导文案（去行内配置）。
- **弹窗头部或顶部工具栏内新增「刷新」按钮**：
  - `variant="secondary"` + `IconRefreshCw`。
  - onClick → `handleRefreshAll()`：调 `fetchBalanceSummary()` 全量重查，成功后整体 `setData(response)`；失败保留旧数据并显示错误提示条。
  - `loading || refreshing` 时禁用。
- 数据态：
  - 顶部 `BalanceSummaryCards`：总余额、可用、冻结、限时、累计调用、累计成本 6 张卡（复用 `Card` / `StatCards` 的视觉语言，不新增风格）。
  - `BalanceCharts`：
    - 柱状图：X 轴为各 API 的 `display_name`，Y 轴为金额（¥）；数据列可选「余额 / 可用 / 冻结 / 限时」分组柱。
    - 环形图：各 API `balanceCny` 占比；余额为 0 的账号归入「0 余额」或忽略，并在图例中体现。
  - `BalanceItemsTable`：明细表列 = 账号、余额、可用、冻结、限时、最近到期、累计调用、累计成本、状态（成功/失败原因）、**操作（单行「余额查询」按钮）**。
- **单行刷新**：`BalanceItemsTable` 每行渲染一个 `Button variant="ghost" size="sm"`（`IconRefreshCw` + `balance_query_item_refresh`）：
  - onClick → `handleRefreshItem(identityId)`：调 `fetchBalanceSummary({ identityId })`。
  - 成功后用返回的 `items[0]` 替换本地 state 中该行，并**本地重算 totals**（按当前 items 累加，或调 `recomputeTotals(items)` 纯函数）；失败则把该行 `error` 更新为错误信息，summary 置空。
  - 请求期间只禁用该行按钮，其他行按钮和全量刷新不受影响（全量刷新与单行刷新互斥：刷新中禁用单行按钮，单行刷新中禁用全量刷新，避免并发覆盖）。
- 失败行：`summary` 为空 + `error` 有值；表格行标红，不参与总计。
- 货币格式化：复用 `Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' })`，注意项目现有 `formatQuotaWindowCost` 是 USD 风格，不要直接套用，余额单独做 CNY 格式化。

`balanceChartConfig.ts`：

- 抽成纯函数 `buildBalanceBarChartData(items)` / `buildBalanceDoughnutChartData(items)` / `recomputeBalanceTotals(items)`，便于 `*.logic.test.ts` 测试。
- 配色从 CSS 变量或主题读取（参考 `AnalysisPanel` 中 `isDark` 传参方式，`BalanceQueryModal` 需要从 `useThemeStore` 取 `isDark`）。

### 5.7 i18n

`web/src/i18n/index.ts` 三语新增 `usage_stats.*` 键：

| key | en | zh | zh-TW |
|---|---|---|---|
| `balance_query` | Balance | 余额查询 | 餘額查詢 |
| `balance_query_modal_title` | TokenRhythm Balance | TokenRhythm 余额 | TokenRhythm 餘額 |
| `balance_query_refresh` | Refresh | 刷新 | 重新整理 |
| `balance_query_item_refresh` | Query | 余额查询 | 餘額查詢 |
| `balance_query_total` | Total Balance | 总余额 | 總餘額 |
| `balance_query_available` | Available | 可用 | 可用 |
| `balance_query_frozen` | Frozen | 冻结 | 凍結 |
| `balance_query_expiring` | Expiring | 限时 | 限時 |
| `balance_query_calls` | Total Calls | 累计调用 | 累計調用 |
| `balance_query_cost` | Total Cost | 累计成本 | 累計成本 |
| `balance_query_next_expiry` | Next Expiry | 最近到期 | 最近到期 |
| `balance_query_empty` | No session configured for TokenRhythm accounts. | 还没有配置余额查询 session。 | 還沒有設定餘額查詢 session。 |
| `balance_query_error` | Failed to load balance summary. | 余额查询失败。 | 餘額查詢失敗。 |
| `balance_query_item_error` | Query failed: {{error}} | 查询失败：{{error}} | 查詢失敗：{{error}} |
| `credentials_balance_session_configure` | Session | 配置 Session | 設定 Session |
| `credentials_balance_session_configured` | Session configured | 已配置 | 已設定 |
| `credentials_balance_session_save_success` | Balance session saved. | Session 已保存。 | Session 已儲存。 |
| `credentials_balance_session_save_failed` | Failed to save balance session. | Session 保存失败。 | Session 儲存失敗。 |
| `credentials_balance_session_clear` | Clear | 清空 | 清空 |
| `credentials_balance_session_hint` | Paste the tr_session cookie value from TokenRhythm browser DevTools. | 粘贴 TokenRhythm 浏览器 DevTools 中的 tr_session Cookie 值。 | 貼上 TokenRhythm 瀏覽器 DevTools 中的 tr_session Cookie 值。 |
| `credentials_balance_session_unsupported` | Balance query only supports TokenRhythm providers. | 余额查询仅支持 TokenRhythm 供应商。 | 餘額查詢僅支援 TokenRhythm 供應商。 |

### 5.8 样式规范

- 新组件样式优先使用现有 SCSS 变量与 mixin（`web/src/styles/variables.scss`、`mixins.scss`），不新造颜色/圆角/阴影体系。
- Modal 沿用全局 `.modal-*` 类；弹窗内部用新 `BalanceQueryModal.module.scss` 管理布局（卡片网格、图表高度、表格）。
- 工具栏按钮高度与 `MainActionButton` 对齐（如 `min-height` 与刷新按钮一致）。
- 暗色模式：图表颜色通过 `isDark` 分支，与 `AnalysisPanel` 相同。
- 移动端：图表堆叠，总计卡片 2 列换 1 列；弹窗宽度 `min(920px, 100%)`。

---

## 6. 交互时序

### 6.1 配置 session

```text
用户点击行内「配置 Session」
  → BalanceSessionModal 打开
  → 输入 tr_session → 保存
  → PATCH /usage/identities/:id/balance-session
  → 后端校验（仅 TokenRhythm 行）→ 写 balance_session
  → 返回 identity（含 balance_session_configured: true）
  → 前端 replaceUsageIdentity → 行内按钮变为「已配置」
  → notice 提示成功
```

### 6.2 查询余额（全量）

```text
用户切到 AI 供应商 tab
  → 刷新按钮左侧出现「余额查询」
  → 点击 → BalanceQueryModal 打开并自动请求
  → POST /usage/balance/query（空 body）
  → 后端过滤 TokenRhythm 且 session 非空的行
  → 并发查询上游 usage-summary
  → 返回 items + totals
  → 弹窗渲染总计模块 + 柱状图 + 环形图 + 明细表
```

### 6.3 弹窗内刷新（全量重查）

```text
用户点击弹窗内「刷新」按钮
  → refreshing = true（旧数据继续展示，按钮禁用）
  → POST /usage/balance/query（空 body）
  → 成功：整体 setData(新数据)
  → 失败：保留旧数据，顶部显示错误提示条
```

### 6.4 单行余额查询（只更新一行）

```text
用户点击明细表某行的「余额查询」按钮
  → refreshingIdentityId = 该行 id（只禁用该行按钮 + 全量刷新按钮）
  → POST /usage/balance/query  body: {"identity_id": "<id>"}
  → 后端只查该行（校验 TokenRhythm + session 非空）
  → 成功：用返回 items[0] 替换该行，本地 recomputeBalanceTotals(items)
  → 失败：该行 summary 置空、error 展示失败原因
```

---

## 7. 测试计划

### 7.1 后端（Go）

| 文件 | 内容 |
|---|---|
| `internal/repository/migration/20260819_add_usage_identity_balance_session_test.go` | 加列幂等、可重复执行 |
| `internal/repository/test/usage_identities_test.go` 或新增 `balance_session_test.go` | 写 session、清空 session、只更新活跃行 |
| `internal/service/balance_service_test.go` | 用 `httptest.Server` 模拟 usage-summary；验证全量过滤（空 session 不查）、单行查询（identityID 命中/未配置 session/非 TokenRhythm 报错）、401 单条失败、并发、总计计算 |
| `internal/api/balance_test.go` | PATCH 路由鉴权/参数校验/不支持类型 400/404；POST query 空 body 返回全量、带 `identity_id` 返回单行、非法 identity_id 400；GET identities 返回布尔值但不含明文 session/base_url |
| `internal/api/usage_identities_test.go` | 更新 DTO 断言（不出现 `balance_session` 值） |
| `internal/service/test/provider_metadata_sync_test.go` | 回归：provider 元数据同步不会清掉手动配置的 `balance_session` |

### 7.2 前端（Vitest）

| 文件 | 内容 |
|---|---|
| `web/src/lib/test/api.test.ts` | 新 API 路径、方法、请求体、错误处理 |
| `web/src/components/usage/credentials/test/credentialViewModels.test.ts` | `buildAiProviderCredentialRows` 不破坏新字段透传 |
| `web/src/components/usage/credentials/AiProviderCredentialsSection.test.tsx` | 仅 `balance_session_supported` 行显示 Session 按钮；已配置态文案 |
| 新 `BalanceSessionModal.test.tsx` | 保存/清空回调、输入校验、关闭 |
| 新 `BalanceQueryModal.test.tsx` | loading/空态/错误态/数据态；打开自动查询；弹窗内「刷新」按钮重查并保留旧数据；单行「余额查询」按钮只更新该行并重算 totals；401 触发 onAuthRequired；mock `react-chartjs-2`（沿用 AnalysisPanel 测试的 mock 方式） |
| 新 `balanceChartConfig.test.ts` | 图表数据聚合、占比计算、0 余额处理、`recomputeBalanceTotals` 单行更新后总计正确 |
| `web/src/pages/test/UsagePage.styles.test.ts` | 更新：`activeTab === 'ai-provider'` 时按钮渲染、`balanceQuerySlot` 位于 `usageRefreshSlot` 前 |
| `web/src/i18n/test/*` | 新增三语 key 存在性 |

---

## 8. 新增/修改文件清单

### 8.1 后端新增

```text
internal/balance/types.go
internal/balance/client.go
internal/service/balance_service.go
internal/api/balance.go
internal/repository/migration/20260819_add_usage_identity_balance_session.go
internal/repository/migration/20260819_add_usage_identity_balance_session_test.go
internal/service/balance_service_test.go
internal/api/balance_test.go
```

### 8.2 后端修改

```text
internal/entities/usage_identity.go
internal/repository/usage_identities.go
internal/repository/migration/migration.go
internal/api/usage_identities.go
internal/api/router.go
internal/app/app.go
```

### 8.3 前端新增

```text
web/src/components/usage/balance/BalanceQueryModal.tsx
web/src/components/usage/balance/BalanceQueryModal.module.scss
web/src/components/usage/balance/BalanceSummaryCards.tsx
web/src/components/usage/balance/BalanceCharts.tsx
web/src/components/usage/balance/BalanceItemsTable.tsx
web/src/components/usage/balance/balanceChartConfig.ts
web/src/components/usage/credentials/BalanceSessionModal.tsx
web/src/components/usage/credentials/BalanceSessionModal.module.scss（如需要）
+ 对应 *.test.tsx / *.test.ts
```

### 8.4 前端修改

```text
web/src/lib/types.ts
web/src/lib/api.ts
web/src/i18n/index.ts
web/src/pages/UsagePage.tsx
web/src/pages/UsagePage.module.scss
web/src/components/usage/credentials/AiProviderCredentialsSection.tsx
web/src/components/usage/credentials/useCredentialsTabData.ts
web/src/components/usage/credentials/useCredentialPages.ts
web/src/components/usage/credentials/CredentialSections.module.scss（行内按钮样式）
```

---

## 9. 实施步骤建议

1. 后端迁移 + 实体 + 仓储（加列、读写 session，先不写 API）。
2. `internal/balance` client + 单测（httptest 覆盖 200/401/非 2xx）。
3. `balance_service` + 单测（过滤、并发、汇总）。
4. API 路由 + DTO 扩展 + 单测。
5. `app.go` 装配，`go test ./...` 回归。
6. 前端类型 + API client + i18n。
7. 数据层（useCredentialPages / useCredentialsTabData）与行内 Session 按钮 + 小弹窗。
8. 余额查询弹窗（总计 + 图表 + 明细表）。
9. UsagePage 工具栏按钮 + 样式 + 响应式。
10. `npm test` / `npm run typecheck` / `npm run lint` 回归；手工验证暗色模式与移动端。

---

## 10. 风险与后续扩展

- **CPA 同步覆盖风险**：`ReplaceUsageIdentitiesForProviderTypes` 必须排除 `balance_session` 列，否则每次元数据同步会清空手动配置（方案已要求，落地必须测试兜底）。
- **TokenRhythm 识别依赖 BaseURL**：若 CPA 中 TokenRhythm 未配置 `base_url`，需改用 `Provider`/`Name` 包含 `tokenrhythm` 判断；落地时先确认实际 CPA 配置。
- **上游限流/超时**：并发 4 + 单账号 8s 超时；若以后账号变多，可升级为与 quota 相同的后台任务 + 轮询模式。
- **session 泄露**：严格禁止 GET 返回明文；PATCH 返回的 identity 也只返回布尔值。
- **后续扩展**：可加「余额阈值告警」「定时刷新余额」「历史余额曲线」等，本次不实现。
