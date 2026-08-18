# 方案：UI 优化 + 高峰/闲时价格编辑完善

## 1. 目标

1. 彻底删除“排名 / Ranking”板块。
2. 为“模型价格设置 → 已保存价格”增加搜索功能。
3. 完善高峰/闲时价格编辑体验：
   - 修复“新增价格”选择 `peak` 后回跳到 `openai` 的问题。
   - 新增价格表单选择 `peak` 时，展示与普通价格字段对齐的 Peak 价格输入行。
   - 每个模型单独配置 Peak 时间（15 分钟粒度下拉选择），取消全局 Peak Hours 弹窗。
   - 已保存价格的编辑弹窗同样支持每个模型单独配置 Peak 时间。
4. 优化设置页：
   - API Key 设置改为统一保存，保存按钮放到 API Key 设置模块右上方。
   - 删除“会话管理”模块。
   - “返回 CPA”按钮放到“退出登录”按钮左侧，并对齐样式。

本次只输出方案，不改代码。

---

## 2. 彻底删除排名板块

### 2.1 前端删除范围

- 删除 `web/src/features/ranking/` 全部文件。
- `web/src/pages/UsagePage.tsx`：
  - 从 `USAGE_TAB_OPTIONS` 移除 `'ranking'`。
  - 删除 ranking 相关 import、state、hooks、数据加载、tab 渲染、scope 切换。
  - 移除 `includeRanking` 参数。
- 删除 `assets/screenshots/ranking-*.png`、`web/src/assets/ranking/`。
- 删除 i18n 中 `ranking.*` 翻译键。
- 删除 ranking 相关测试。

### 2.2 后端删除范围

- 删除 `internal/ranking/`、`internal/ranking/httpapi/`。
- 删除 ranking 路由注册与 `internal/api` 下相关测试。
- 删除 `internal/app` 中 ranking service / runner / HTTP client 装配。
- 删除 `internal/entities/local_ranking_period_stat.go`。
- 保留历史 migration 文件，新增 `drop_ranking_tables` migration 删除 ranking 表。
- 删除 ranking 相关配置项和文档说明。

### 2.3 数据库

新增 migration：

```text
20260818_drop_ranking_tables
```

执行：

- 删除 `local_ranking_period_stats`
- 删除其它 ranking 相关表
- 保留 `schema_migrations` 历史记录，保证迁移链完整

---

## 3. 已保存价格搜索功能

### 3.1 现状

- `PriceSettingsCard.tsx` 的“已保存价格”列表没有搜索框。
- 模型很多时难以滚动查找。

### 3.2 设计

- 在“已保存价格”区域顶部加搜索框。
- 不区分大小写，按模型名子串过滤。
- 支持清空恢复、无结果空状态。
- 纯前端过滤，不请求后端。

```ts
const filteredSavedPrices = useMemo(() => {
  const keyword = savedPriceSearch.trim().toLowerCase();
  if (!keyword) return sortedModelPrices;
  return sortedModelPrices.filter((entry) =>
    entry.model.toLowerCase().includes(keyword),
  );
}, [sortedModelPrices, savedPriceSearch]);
```

---

## 4. 高峰/闲时价格编辑完善

### 4.1 修复新增价格选择 Peak 回跳 OpenAI

现状问题：

- 新增价格表单的“计价风格”下拉选择 `peak` 后，事件处理里把非 `claude` 的值统一映射成了 `openai`。
- 导致 `peak` 无法保持。

方案：

- 在新增价格表单中，`onChange` 显式处理三种值：

```ts
const handleStyleChange = (value: string) => {
  if (value === 'claude') setPricingStyle('claude');
  else if (value === 'peak') setPricingStyle('peak');
  else setPricingStyle('openai');
};
```

- 新增价格提交时：
  - `peak` 只作为 UI 状态，不直接发送给后端。
  - 后端仍使用 `openai` 作为 `pricing_style`。
  - Peak 价格作为基础价格写入。
  - 同时写入该模型的 Peak 时间配置。

### 4.2 新增价格表单：选择 Peak 后展示 Peak 价格输入行

设计：

- 当前新增价格表单已有：
  - Prompt
  - Completion
  - Cache Read
  - Cache Write
  - Multiplier
- 当计价风格选择 `peak` 时，在普通价格字段下方新增一行 **Peak 价格** 输入：
  - Peak Prompt
  - Peak Completion
  - Peak Cache Read
  - Peak Cache Write
- 普通价格字段在 Peak 模式下语义变为 **Off-peak 价格**。
- 两行输入框宽度、间距、标签样式保持对齐。
- 表单校验：
  - Peak 价格必须 >= 0。
  - 若 Peak Prompt > 0，Off-peak Prompt 不能大于 Peak Prompt？不强制，但保存时会按比例计算 multiplier。
  - 如果 Peak 价格为 0 且 Off-peak 价格非 0，提示用户无法计算倍率。

### 4.3 每个模型单独配置 Peak 时间（替代全局 Peak Hours）

现状问题：

- 当前 Peak Hours 是全局配置，放在 `PeakHoursModal` 中。
- 用户希望每个模型单独配置自己的 Peak 时间。

方案：

- **移除全局 Peak Hours 弹窗与 `pricing.peak_hours` 全局配置入口。**
- 在模型价格配置中增加“Peak Time”配置，跟随模型保存。

#### 数据模型

在 `model_price_settings` 增加字段：

```text
peak_hours_config TEXT NULL
```

存储 JSON：

```json
{
  "timezone": "Asia/Shanghai",
  "ranges": [
    { "start": "09:00", "end": "12:00" },
    { "start": "14:00", "end": "18:00" }
  ]
}
```

- 未配置时：该模型不区分高峰/闲时，全部按基础价格计价。
- 配置后：Resolver 根据该模型的 Peak 时间判断 `peak` / `off_peak`。
- 删除全局 `pricing.peak_hours` app setting 的读取逻辑。

#### 后端改动

- `entities.ModelPriceSetting` 增加 `PeakHoursConfig *string` 或 JSON 字段。
- repository / service / API：
  - `GET /api/v1/pricing` 返回 `peak_hours_config`。
  - `PUT /api/v1/pricing/:model` 接收 `peak_hours_config`。
- Resolver：
  - 优先使用模型自身 `PeakHoursConfig`。
  - 模型未配置时视为全 `peak`。
- 删除 `PeakHoursModal` 相关前端组件与全局 API：
  - `GET/PUT /api/v1/pricing/peak-hours` 不再需要。
- 保留 `pricing.PeakHoursConfig` 类型与解析逻辑，作为模型级配置使用。

#### 前端 UI：Peak Time 下拉选择

- 时间粒度：**15 分钟**。
- 时间选项：
  - Start / End 各一个下拉框。
  - 选项如 `00:00`、`00:15`、`00:30` ... `23:45`。
- 支持多条区间：
  - 默认一条 `09:00 - 12:00`。
  - 可添加 / 删除区间。
- UI 对齐：
  - 与上方“模型选择 / 计价风格”选择栏对齐。
  - 宽度平分，使用相同 Select 组件。
- 校验：
  - Start 不能等于 End。
  - 支持跨午夜，例如 `22:00 - 02:00`。
  - 区间重叠允许，但保存前做归一化合并。

### 4.4 已保存价格的编辑弹窗也支持单独配置 Peak Time

- 在已保存价格模型点击“编辑”打开的弹窗中：
  - 同样支持计价风格选择 `peak`。
  - 显示 Peak 价格输入行。
  - 显示该模型自己的 Peak Time 配置。
- 保存时：
  - 更新模型基础价格（Peak 价格）。
  - 更新该模型的 `peak_hours_config`。
  - 根据 Off-peak / Peak 输入计算 `off_peak` multiplier 并写入规则。

### 4.5 前端组件调整

- 删除 `PeakHoursModal.tsx` / `PeakHoursModal.module.scss`。
- 新增或重构 `PeakTimeEditor` 组件：
  - 15 分钟粒度 Start / End 下拉。
  - 支持多条区间。
  - 可复用于新增价格表单和编辑弹窗。
- `PriceSettingsCard.tsx`：
  - 新增价格表单和编辑弹窗都接入 `PeakTimeEditor`。
  - 移除全局 “Peak hours” 按钮。
- i18n 新增：
  - `peak_time`
  - `peak_time_start`
  - `peak_time_end`
  - `peak_time_add`
  - `peak_time_remove`
  - `peak_time_invalid`

---

## 5. API Key 设置统一保存

### 5.1 现状

- 每个 API Key 模块内部有自己的保存按钮。
- 编辑多个 API Key 时，需要逐个保存。

### 5.2 目标

- 将保存按钮从每个 API Key 模块内部移除。
- 在 API Key 设置模块右上角放一个统一的“保存”按钮。
- 编辑多个 API Key 后，点击一次统一保存，批量提交。

### 5.3 设计

- 页面状态：
  - 每个 API Key 编辑项进入“草稿”状态。
  - 修改后标记为 `dirty`。
- 右上角保存按钮：
  - 有 dirty 项时可用。
  - 点击后批量提交所有 dirty 项。
  - 提交结果按项展示成功/失败。
- 取消 / 离开：
  - 如果有未保存 dirty 项，提示确认。
  - 确认后丢弃草稿。
- 后端：
  - 如果已有批量 API Key 更新接口则复用。
  - 否则新增批量更新 API。
- 测试：
  - 多个模块编辑后统一保存。
  - 部分失败时的提示与回滚策略（不自动回滚已成功项，但明确展示失败项）。

---

## 6. 删除设置页“会话管理”模块

- 定位设置页中“会话管理 / Session Management”模块。
- 删除：
  - 前端模块组件、样式、翻译键。
  - 相关 API 调用。
  - 相关测试。
- 如果后端有只服务于该模块的接口，评估是否一并删除或保留兼容。
- 不删除登录/退出登录本身。

---

## 7. “返回 CPA”按钮位置调整

- 当前“返回 CPA”按钮位置需要调整。
- 目标：
  - 放到“退出登录”按钮左侧。
  - 与“退出登录”按钮对齐，高度、间距、样式保持一致。
- 实现：
  - 调整设置页头部/用户菜单布局。
  - 使用相同 Button 尺寸与变体。
  - 保留原有跳转逻辑。

---

## 8. 代码规范

### 8.1 总体原则

- 遵循现有前端/后端分层与命名风格。
- 保持向后兼容：未配置 Peak 时间的模型行为不变。
- 不为了“优雅”过度抽象；组件复用优先。
- 每个新组件/接口补充注释，说明“为什么”。

### 8.2 Go 后端规范

- 使用强类型：
  - `PeakHoursConfig` 继续使用现有 `internal/pricing/peak_hours.go` 类型。
  - 不把 JSON 字符串散落到业务代码。
- Repository 层：
  - 新增字段显式迁移，不依赖 GORM AutoMigrate 隐式改表。
  - migration 幂等：先检查列/索引是否存在。
- Service 层：
  - 校验 Peak Time 配置后再写入。
  - 错误使用哨兵错误包装。
- Resolver：
  - 优先使用模型级 PeakHoursConfig。
  - 模型级未配置时回退为全 Peak，避免历史数据成本突变。
- 测试：
  - 覆盖模型级 Peak Time 边界、跨午夜、未配置、非法配置。
  - 确保 `go test ./...` 通过。

### 8.3 前端规范

- 保持现有 React + TypeScript 结构。
- 新增表单状态使用 `useState` / `useMemo`，不引入重型状态库。
- Peak Time 下拉选项生成：

```ts
const TIME_OPTIONS = Array.from({ length: 96 }, (_, index) => {
  const hour = Math.floor(index / 4);
  const minute = (index % 4) * 15;
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
});
```

- 组件拆分：
  - `PeakTimeEditor` 负责时间区间编辑。
  - `PriceSettingsCard` 负责表单组合与提交。
- 表单校验集中到独立函数，避免内联逻辑散落。
- i18n 文案不硬编码。
- 样式尽量复用现有 SCSS 变量和组件类，避免新增大段重复样式。

### 8.4 边界问题

1. Peak 时间未配置：模型全时段按 Peak 价，等价于旧行为。
2. Start == End：校验拒绝。
3. 跨午夜：如 `22:00 - 02:00` 必须正确判断。
4. 多条区间重叠：保存前合并/归一化。
5. Peak 价格为 0 且 Off-peak 价格 > 0：无法计算 multiplier，应提示用户。
6. Off-peak / Peak 比例不一致：当前后端按统一 multiplier 实现；如果用户填写的各字段比例不同，需要提示或限制为统一倍率。
7. API Key 统一保存：部分失败时保留已成功结果并明确展示失败项。
8. 删除会话管理模块：不影响登录态和退出登录。
9. 返回 CPA 按钮调整：保持原有跳转逻辑，不改变 URL 生成规则。

---

## 9. 实施顺序建议

1. 先实现低风险 UI 调整：
   - 删除会话管理模块。
   - 调整“返回 CPA”按钮位置。
2. 实现已保存价格搜索。
3. 修复新增价格 Peak 回跳问题，并补充 Peak 价格输入行。
4. 将全局 Peak Hours 改为模型级 Peak Time：
   - 后端数据模型 + migration
   - Resolver 逻辑
   - API 调整
   - 前端 PeakTimeEditor
5. API Key 统一保存。
6. 删除 Ranking 板块。
7. 运行完整测试：
   - `go test ./...`
   - `npm --prefix ./web run typecheck`
   - `npm --prefix ./web run lint`
   - `npm --prefix ./web test -- --run`
