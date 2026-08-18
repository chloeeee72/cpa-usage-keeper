# CPA Usage Keeper 高峰/闲时定价实现方案

## 1. 背景与目标

当前 CPA Usage Keeper 的 `model_price_settings` 只有一套价格（`prompt / completion / cache_read / cache_creation`），无法表达 DeepSeek 等厂商的“高峰/闲时”两套价格，也无法按请求时间自动选择价格。

目标：

1. 支持为每个模型配置高峰、闲时两套价格（或等价的高峰/闲时倍率）。
2. 支持配置高峰时段（默认 DeepSeek 官方：北京时间 `09:00-12:00`、`14:00-18:00`）。
3. 成本计算必须基于**请求实际发生时间**选择价格，而不是只看模型名。
4. 保证历史数据、聚合数据、实时数据都能得到一致且正确的成本。

## 2. 现状代码关键路径

| 模块 | 文件 | 说明 |
|---|---|---|
| 价格实体 | `internal/entities/model_price_setting.go` | 单套价格 + `price_multiplier` |
| 价格规则实体 | `internal/entities/model_price_rule.go` | 按 usage 维度匹配的倍率规则 |
| 价格快照 | `internal/pricing/snapshot.go` | 编译并校验整份价格目录 |
| 价格解析器 | `internal/pricing/resolver.go` | 根据 `CostSubject` 计算成本 |
| 规则字段 | `internal/pricing/fields.go` | 目前支持 `api_group_key/model/auth_index/model_alias/service_tier/...` |
| 计价输入 | `internal/repository/usage_pricing_subject.go` | 把 event/record/聚合行转成 `CostSubject` |
| 成本公式 | `internal/helper/usage_cost.go` | 四段 token 成本计算 |
| Overview 聚合 | `internal/overview/aggregate.go` | 按维度聚合成 hourly/daily rows |
| Overview 查询 | `internal/repository/usage_overview_pricing.go` | 用聚合行 + resolver 计算成本 |
| 窗口统计 | `internal/repository/usage_window_stats.go` | 对时间窗口内的 hourly 聚合再汇总 |
| 应用配置 | `internal/entities/app_setting.go` | 可用于保存高峰时段 JSON 配置 |

## 3. 总体设计

采用**“规则字段 + 聚合维度”**方案：

- 新增一个 usage 维度 `pricing_period`，取值固定为 `peak` / `off_peak`。
- 新增一个价格规则字段 `pricing_period`，复用现有 `model_price_rules` 的倍率能力。
- 在 `app_settings` 中保存高峰时段配置。
- `Resolver` 根据请求时间 + 高峰时段配置计算 `pricing_period`，再应用对应规则倍率。
- 聚合表（hourly/daily）增加 `pricing_period` 维度，保证跨高峰/闲时边界的 bucket 不会被错误地整体算成同一个价格。

这种设计的好处：

- 最大限度复用现有价格快照、规则校验、快照发布机制。
- 不破坏现有单套价格行为：未配置任何 `pricing_period` 规则时，所有请求按基础价格（可视为高峰价）计算。
- 高峰/闲时本质上是一个“按时间维度打折/加价”的规则，与现有 `service_tier`、`reasoning_effort` 规则模型一致。

## 4. 详细改动

### 4.1 高峰时段配置

在 `app_settings` 新增一个 JSON 配置项：

```json
{
  "setting_key": "pricing.peak_hours",
  "value_type": "json",
  "value": {
    "timezone": "Asia/Shanghai",
    "ranges": [
      { "start": "09:00", "end": "12:00" },
      { "start": "14:00", "end": "18:00" }
    ]
  }
}
```

边界与校验：

- `timezone` 必须能被 `time.LoadLocation` 识别，默认 `Asia/Shanghai`。
- `ranges` 支持跨午夜，例如 `22:00-02:00`。
- 同一配置内区间允许重叠，但编译时应做归一化/合并，避免同一条请求匹配多个区间导致歧义。
- 区间采用 `[start, end)` 半开区间，避免 `12:00` 同时属于两个区间。
- 未配置或配置为空时，所有请求视为 `peak`（保持向后兼容）。
- 配置变更后需要重新编译并发布价格快照，同时触发 overview 聚合重建（见 4.5）。

### 4.2 新增 `pricing_period` 规则字段

修改 `internal/pricing/fields.go`：

- 在 `RuleField` 枚举中新增：

```go
RuleFieldPricingPeriod
```

- 在 `ruleFieldNames` 中注册：

```go
RuleFieldPricingPeriod: "pricing_period"
```

- 在 `UsageDimensions` 中新增字段：

```go
PricingPeriod string
```

- 在 `Value()` 中返回 `PricingPeriod`。

`pricing_period` 的合法值建议限定为 `peak` / `off_peak`，由规则编译阶段校验，避免任意字符串污染规则。

### 4.3 `CostSubject` 增加时间信息

修改 `internal/pricing/resolver.go` 的 `CostSubject`：

```go
type CostSubject struct {
    Timestamp  time.Time
    Dimensions UsageDimensions
    Tokens     helper.UsageTokenCostInput
}
```

`NewCostSubject` 增加 `timestamp` 参数；现有调用点由 repository 层从 event/record/聚合行传入。

### 4.4 Resolver 计算流程

`Resolver.Calculate` 中：

1. 如果 `Dimensions.PricingPeriod` 非空，直接使用该值（聚合行场景）。
2. 否则根据 `Timestamp` + 高峰时段配置计算 `pricing_period`。
3. 将 `pricing_period` 写入一份 `UsageDimensions` 副本。
4. 按现有逻辑匹配模型和规则。
5. 如果命中 `pricing_period` 规则，则应用对应倍率。

示例规则配置（通过现有 `/api/v1/pricing/rules` API）：

```json
{
  "model": "Deepseek-deepseek-v4-flash",
  "rules": [
    { "key": "pricing_period", "value": "peak",    "multiplier": 1.0 },
    { "key": "pricing_period", "value": "off_peak", "multiplier": 0.5 }
  ]
}
```

如果后续希望直接编辑“两套绝对价格”，可以在 `model_price_settings` 增加 `off_peak_*` 列，或由前端把“闲时价格 / 高峰价格”自动换算成 `off_peak` 倍率。推荐先实现规则倍率，UI 上提供“高峰价 / 闲时价”输入框并自动换算。

### 4.5 Overview 聚合支持按 `pricing_period` 拆分

当前 `usage_overview_hourly_stats` / `usage_overview_daily_stats` 的 unique index 包含 10 个维度，没有时间维度，导致一个小时/一天内跨高峰与闲时的 token 会混在同一行，无法准确计价。

需要：

1. 给两张表增加列：

```sql
pricing_period TEXT NOT NULL DEFAULT 'peak'
```

2. 在 `aggregateKey` 中加入 `PricingPeriod`。
3. 在 `overview.BuildRows` 中根据每条 event 的 `Timestamp` 计算 `pricing_period`。
4. 更新 unique index，把 `pricing_period` 加入维度：
   - `uniq_usage_overview_hourly_stats_dimensions`
   - `uniq_usage_overview_daily_stats_dimensions`
5. 迁移策略参考现有 `20260723_usage_overview_five_dimensions.go`：
   - 加列
   - 删除旧唯一索引
   - 清空 hourly/daily 聚合表
   - 重置 overview checkpoint
   - 从 `usage_events` 全量重建聚合
   - 创建新唯一索引

### 4.6 各成本计算路径更新

| 路径 | 改动 |
|---|---|
| 原始事件列表 | `UsageEventCostSubject` 传入 `event.Timestamp` |
| 实时事件 | 同上 |
| hourly/daily 聚合行 | `UsageOverviewHourlyCostSubject` / `DailyCostSubject` 读取行内 `PricingPeriod`，并作为 dimension 传入 |
| Overview 统计 | 查询时按 `pricing_period` 分组，或把同一 bucket 的两行分别计价后加总 |
| 窗口统计 `usage_window_stats` | 当前会跨 period 合并 token；需要改为按 `pricing_period` 分组后分别计价再加总，避免把高峰/闲时 token 混在一起按同一价格计算 |
| Analysis 投影 | 同步在投影 SQL/Go 聚合中带上 `pricing_period` |

### 4.7 API 与前端

后端新增/调整：

- `GET /api/v1/pricing/peak-hours`：读取当前高峰时段配置。
- `PUT /api/v1/pricing/peak-hours`：保存高峰时段配置。
- `PUT /api/v1/pricing/rules`：已存在，支持 `pricing_period` 规则。
- `GET/PUT /api/v1/pricing`：如需直接编辑两套绝对价格，再扩展 `off_peak_*` 字段。

前端：

- `PriceSettingsCard` 增加“高峰/闲时价格”编辑入口。
- 新增“高峰时段设置”弹窗/表单。
- `PriceRulesModal` 增加 `pricing_period` 规则展示与编辑。
- 价格预览/成本展示区分高峰、闲时。

## 5. 边界问题与处理

1. **时间边界**
   - 采用 `[start, end)` 半开区间。
   - 跨午夜区间需正确处理。
   - 使用配置的 `timezone` 计算本地时间，不能用服务器本地时区硬编码。

2. **未配置高峰时段**
   - 默认全部按 `peak` 计价，等价于当前行为，避免升级后历史成本突变。

3. **未配置 `pricing_period` 规则**
   - 所有请求都按基础价格计算，不需要用户必须配置规则。

4. **高峰/闲时规则缺失其一**
   - 例如只配置 `off_peak` 倍率，未配置 `peak`：`peak` 请求按基础价（倍率 1），`off_peak` 按配置倍率。
   - 只配置 `peak`：`off_peak` 也按基础价，等价于没有区分。

5. **聚合 bucket 跨时段**
   - 小时桶如 `09:00-10:00` 完全在高峰内；`12:00-13:00` 完全在闲时内；`08:00-09:00` 完全闲时。
   - 真正跨边界的只有边界小时，例如 `08:00-09:00` 若高峰从 09:00 开始，桶内 09:00 整点事件属于高峰；由于聚合按 `pricing_period` 拆分，天然正确。
   - 如果不拆分，仅用 `BucketStart` 判断会把整个小时算错。

6. **窗口统计**
   - 跨小时/跨天查询时，必须按 `pricing_period` 分组，否则 token 混在一起后无法正确计价。

7. **价格倍率溢出**
   - 复用现有 `validateWorstCaseCost` 的防溢出校验。
   - `pricing_period` 规则倍率同样参与 worst-case 校验。

8. **规则冲突**
   - 同一模型同一 `pricing_period` 值只能有一条规则，否则编译报错。
   - 同一请求只会命中一个 `pricing_period` 值，不会同时命中 `peak` 和 `off_peak`。

9. **历史数据重算**
   - Overview 聚合表需要重建。
   - 原始 `usage_events` 保留完整时间戳，因此历史成本可以重算。
   - 若用户只关心新数据，也可以提供“仅从某个时间点开始按新规则计价”的选项。

10. **时区/DST**
    - 使用 IANA 时区名，支持未来有 DST 的地区。
    - 高峰时段按本地墙钟时间判断，而不是 UTC 固定偏移。

## 6. 迁移与发布步骤

1. 新增 migration：
   - `app_settings` 写入默认 `pricing.peak_hours`（若不存在）。
   - `model_price_settings` 不必须变更（规则方案）。
   - `usage_overview_hourly_stats` / `usage_overview_daily_stats` 增加 `pricing_period` 列。
   - 重建 unique index 并清空/重建 overview 聚合。
2. 后端：
   - `fields.go` 增加 `pricing_period`。
   - `resolver.go` 支持时间计算 period。
   - `usage_pricing_subject.go` 传入 timestamp / period。
   - `overview` 聚合按 period 拆分。
   - 新增 peak-hours API 与 service。
3. 前端：
   - 高峰时段配置 UI。
   - 价格规则/价格编辑 UI 支持 `pricing_period`。
4. 测试：
   - 单元测试：时段判断边界、跨午夜、时区、规则匹配、聚合拆分。
   - 集成测试：事件入库 -> overview 聚合 -> 页面成本展示。
   - 迁移测试：旧库升级后数据重建正确。
   - 回归测试：未配置任何 peak/off-peak 时成本与旧版本一致。

## 7. 备选方案：直接两套价格列

如果产品上更希望用户直接填“高峰输入价、闲时输入价、高峰输出价、闲时输出价……”，可以在 `model_price_settings` 增加：

```text
off_peak_prompt_price_per1_m
off_peak_completion_price_per1_m
off_peak_cache_read_price_per1_m
off_peak_cache_creation_price_per1_m
```

`Resolver` 根据 `pricing_period` 选择使用基础价格还是 off-peak 价格。这个方案更直观，但改动面更大（实体、DTO、API、前端、迁移、校验）。建议作为规则方案的后续增强，而不是第一版。

## 8. 不做的事

- 不修改 CPA 本身。
- 不改变现有 token 用量采集逻辑。
- 不引入外部定价服务。
- 第一版不实现“按天/按星期自定义高峰时段”，只实现统一的每日高峰时段；但配置结构可扩展 `days` 字段。

## 9. 代码规范

实现时严格遵循本仓库已有风格，保持代码优雅、可读、可维护。

### 9.1 总体原则

- 遵循现有分层：`entities` 只放数据模型，`repository` 只做持久化，`service` 负责业务与校验，`api` 只做 HTTP 适配，`pricing` 负责纯领域计算。
- 保持向后兼容：未配置高峰/闲时时，行为与当前版本完全一致。
- 新功能尽量复用现有机制（价格快照、规则编译、resolver、app_settings、migration），不重复造轮子。
- 不引入不必要的第三方依赖；能用标准库 `time` 解决的时区/区间问题就不用额外库。

### 9.2 Go 代码风格

- 命名遵循 Go 惯例：
  - 类型：`PeakHoursConfig`、`PricingPeriod`、`PricingPeriodResolver`
  - 常量：`PricingPeriodPeak = "peak"`、`PricingPeriodOffPeak = "off_peak"`
  - 配置 key：`AppSettingPricingPeakHours = "pricing.peak_hours"`
- 使用强类型而不是裸字符串：
  - 定义 `type PricingPeriod string`
  - 定义 `type PeakHoursConfig struct` 和 `type PeakTimeRange struct`
  - 避免在业务代码里散落 `"peak"` / `"off_peak"` 字面量
- 函数保持短小、单一职责；超过 30-50 行的函数应拆分。
- 使用 early return 减少嵌套。
- 错误处理沿用现有方式：
  - 内部错误用 `fmt.Errorf("...: %w", err)` 包装
  - 业务错误使用 `errors.New` 定义哨兵错误，例如 `ErrInvalidPeakHoursConfig`
  - API 层统一映射为 400/404/500，不把内部错误细节直接暴露给前端
- 不写无法解释的魔法数字/字符串；时间格式、区间边界等抽成常量或配置结构。
- 并发安全沿用现有模式：
  - 价格快照继续用 `atomic.Pointer` 不可变发布
  - 写操作继续通过 `mutationMu` 串行化
  - 高峰时段配置变更后，先编译完整新快照，再原子替换，禁止半更新状态

### 9.3 时间与时段处理

- 所有时间判断使用配置的 IANA 时区，例如 `time.LoadLocation("Asia/Shanghai")`，禁止直接依赖服务器本地时区。
- 区间采用 `[start, end)` 半开区间，统一用分钟或 `time.Time` 比较，避免字符串比较。
- 跨午夜区间必须单独处理。
- 高峰时段配置解析、校验、归一化收敛到一个文件/类型中，例如 `internal/pricing/peak_hours.go`。
- 为 `PeakHoursConfig` 提供：
  - `Validate() error`
  - `Normalize() PeakHoursConfig`
  - `IsPeak(t time.Time) bool`

### 9.4 数据与迁移

- 新列名、唯一索引名沿用现有命名风格：
  - 列：`pricing_period`
  - 索引：`uniq_usage_overview_hourly_stats_dimensions` / `uniq_usage_overview_daily_stats_dimensions`
- migration 必须幂等：
  - 加列前检查 `HasColumn`
  - 建索引前检查 `HasIndex`
  - 重建 overview 时先清空聚合表、重置 checkpoint，再分批从 `usage_events` 重建
- 不使用 `Save` 做无关更新；保持 repository 层 SQL 字段显式。
- DTO 转换保持显式字段映射，不依赖反射或 map。

### 9.5 测试规范

- 使用 table-driven tests，覆盖：
  - 时段边界：`09:00`、`12:00`、`14:00`、`18:00`
  - 跨午夜：`22:00-02:00`
  - 时区：同一 UTC 时间在不同时区属于不同 period
  - 规则缺失、重复、非法值
  - 聚合拆分：同一小时跨 peak/off_peak 的两行成本正确
  - 窗口统计：跨 period 合并后成本正确
  - 迁移：旧库升级后 overview 重建正确
- 测试命名沿用现有风格，例如 `TestPeakHoursConfigIsPeak`、`TestResolverPricingPeriodRule`。
- 尽量使用真实 SQLite + repository 做集成测试，纯逻辑部分才使用轻量单元测试。

### 9.6 前端代码风格

- 沿用现有 React + TypeScript 结构：
  - 新增 API 调用放在 `web/src/lib` 下
  - 页面状态逻辑放在 `web/src/components/usage/hooks`
  - UI 组件放在 `web/src/components/usage/pricing` 或对应目录
- 组件保持单一职责，表单校验放在独立函数中。
- i18n 文案补充到现有语言文件，不硬编码中文/英文到组件里。
- 高峰期配置表单需要前端预校验：
  - 时间格式必须是 `HH:MM`
  - 开始时间不能等于结束时间（除非显式表示 24 小时）
  - 跨午夜区间给出明确提示

### 9.7 可读性与可维护性

- 每个新增公开类型/函数写简短注释，说明“为什么”而不是“是什么”。
- 高峰时段配置示例写入 `README` 或 `doc`，方便后续维护。
- 涉及价格计算的关键路径，保留与 `helper.CalculateUsageTokenCostBreakdown` 一致的注释风格。
- 不为了“优雅”而过度抽象；如果规则方案已经足够，就不引入额外接口层。
- 代码提交前运行：
  - `go test ./...`
  - `npm --prefix ./web run lint`
  - `npm --prefix ./web run typecheck`
  - `npm --prefix ./web run test`
