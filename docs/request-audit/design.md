# 请求详细审计设计稿

## 1. 背景与目标

本功能用于在`自用模式`或`演示模式`下，为 AI 请求调试提供完整、可追溯的上下文。

设计目标：

- 尽可能记录一次请求的完整调用信息
- 不破坏现有 `Router -> Controller -> Service -> Model` 分层
- 不污染现有消费/错误日志结构
- 与现有前端日志入口兼容
- 尽量减少与上游后续同步时的冲突面

非目标：

- 不替代现有 `model.Log` 消费统计能力
- 不把二进制文件内容长期存入数据库
- 不在对外运营模式下默认开启

## 2. 已确认的现有实现边界

### 2.1 模式开关

当前项目没有单独的 `mode` 枚举，而是通过两个布尔值组合出三种模式：

- `SelfUseModeEnabled = true`：自用模式
- `DemoSiteEnabled = true`：演示模式
- 两者都为 `false`：对外运营模式

因此，请求审计的启用条件定义为：

```text
operation_setting.SelfUseModeEnabled || operation_setting.DemoSiteEnabled
```

### 2.2 日志权限模型

现有日志体系已经是“双视角”：

- 普通用户访问 `/api/log/self`
- 管理员访问 `/api/log/`

前端 `使用日志` 页面也是基于同一页面按角色切换请求接口。

这意味着新审计功能最稳妥的权限策略是：

- 普通用户：查看自己的审计详情
- 管理员：查看全量审计详情

### 2.3 现有前端入口

前端当前至少有三类日志页：

- `使用日志`：标准 relay 消费/错误日志
- `绘图日志`：Midjourney 日志
- `任务日志`：Suno / 视频等任务日志

因此“全部请求都能查看审计详情”的前端落点不应只挂在 `使用日志`，而应将审计详情能力做成可复用组件，分别接入：

- `web/src/components/table/usage-logs`
- `web/src/components/table/mj-logs`
- `web/src/components/table/task-logs`

## 3. 范围定义

### 3.1 纳入记录的请求范围

本次设计覆盖以下入口：

1. 标准 relay
   - `/v1/chat/completions`
   - `/v1/completions`
   - `/v1/messages`
   - `/v1/responses`
   - `/v1/responses/compact`
   - `/v1/embeddings`
   - `/v1/images/*`
   - `/v1/audio/*`
   - `/v1/realtime`
   - `/v1beta/models/*`

2. Playground
   - `/pg/chat/completions`

3. Task / 视频 / Suno
   - `/suno/*`
   - `/v1/video/generations`
   - `/v1/videos`
   - `/v1/videos/:video_id/remix`
   - task fetch 类查询接口

4. Midjourney
   - `/mj/submit/*`
   - `/mj/task/*`
   - `/:mode/mj/*`
   - `notify` / fetch / image-seed 等

### 3.2 不保存原文的内容

以下内容不保存原文，只保存元信息：

- 上传文件
- 图片原始二进制
- 音频原始二进制
- 视频原始二进制
- base64 大字段
- 二进制下载响应体

元信息包括但不限于：

- 字段名
- 文件名
- MIME 类型
- 大小
- 来源类型（upload/url/base64/data-url）
- 摘要标识（可选，如 sha256）

### 3.3 流式与实时响应

流式和实时接口采用“聚合后的最终结果”策略：

- 保存最终聚合后的文本/结构化结果
- 保存 usage、finish_reason、首包时间、总耗时、错误信息、重试链路
- 不保存每个 SSE chunk
- 不保存每个 websocket frame

## 4. 核心设计

## 4.1 总体方案

采用“双层日志”：

1. 现有 `model.Log`
   - 继续承担列表索引、统计、筛选、运营展示
   - 不存完整请求/响应大文本

2. 新增 `RequestAudit`
   - 独立存放完整审计详情
   - 通过 `request_id` 与现有日志行关联

这样做的优点：

- 现有日志表改动小
- 新功能边界清晰
- 前端可以继续从原列表进入详情
- 后端存储与清理策略独立

## 4.2 新增模型建议

建议新增独立模型：

```text
model/request_audit.go
service/requestaudit/*
controller/request_audit.go
```

表名建议：

```text
request_audits
```

建议字段：

### 基础索引字段

- `id`
- `created_at`
- `updated_at`
- `request_id`
- `user_id`
- `username`
- `mode`
- `route_group`
- `route_path`
- `method`
- `status_code`
- `success`

### 调用定位字段

- `relay_format`
- `relay_mode`
- `is_stream`
- `is_playground`
- `model_name`
- `upstream_model_name`
- `group_name`
- `token_id`
- `token_name`
- `channel_id`
- `channel_name`
- `channel_type`

### 性能字段

- `started_at`
- `finished_at`
- `latency_ms`
- `first_response_ms`
- `retry_count`

### 大块内容字段

- `request_payload`
- `response_payload`
- `trace_payload`

字段类型建议：

- 关系字段和索引用普通列
- 大块结构体使用 `TEXT` 存 JSON 字符串
- 不使用数据库方言专属 JSONB，确保 SQLite / MySQL / PostgreSQL 兼容

## 4.3 Payload 结构建议

### request_payload

建议保存为 JSON 对象，包含：

- `headers`
- `query`
- `body_kind`
- `body_text`
- `body_json`
- `body_form`
- `body_files`
- `sanitized_fields`

说明：

- 文本请求优先保留原始文本和 JSON 快照
- 文件类请求保留字段与元信息
- 识别为二进制/大 base64 的字段，替换为元信息占位对象

### response_payload

建议保存：

- `headers`
- `body_kind`
- `body_text`
- `body_json`
- `body_files`
- `aggregated_stream_text`
- `aggregated_stream_json`
- `binary_meta`

说明：

- 普通 JSON 响应优先保留原文与 JSON
- 流式响应保存聚合结果
- 二进制响应仅保存元信息

### trace_payload

建议保存：

- `request_conversion_chain`
- `final_request_relay_format`
- `attempts`
- `billing`
- `usage`
- `error`
- `context`

其中：

- `attempts`：每次重试的渠道、状态、错误
- `billing`：预扣费、结算、退款、订阅信息
- `usage`：prompt/completion/audio/image/tool usage 等
- `context`：reasoning_effort、playground、auto-group、task/mj 关联信息等

## 5. 敏感信息策略

本功能虽然运行在自用/演示模式，但仍应保留基础脱敏能力，避免把真正的密钥原文落库。

### 5.1 强制脱敏字段

请求头、查询参数、上下文中出现以下内容时，不保存原值：

- `Authorization`
- `Cookie`
- `x-api-key`
- `x-goog-api-key`
- `mj-api-secret`
- 上游渠道密钥
- 用户 token key
- query 中的 `key`

替代策略：

- 保存脱敏值，如 `sk-****abcd`
- 在可行时补充名字型信息，如 `token_name`、`channel_name`

### 5.2 明文保留字段

以下文本内容按需求明文保留：

- `prompt`
- `messages`
- `input`
- 一般文本参数
- 一般文本响应内容
- 错误信息

## 6. 采集链路设计

## 6.1 标准 relay / playground

建议在 `controller/relay.go` 增加独立审计生命周期：

1. 请求解析后初始化审计草稿
2. 从 `BodyStorage` 读取请求快照
3. 生成 `RelayInfo` 后补齐上下文
4. 每次重试记录一次 `attempt`
5. 请求完成后记录响应聚合结果
6. 成功/失败统一落库

原因：

- `RelayInfo` 已聚合大量上下文
- 主链已包含 request_id、计费、重试、渠道选择
- 侵入点集中

## 6.2 Task / 视频 / Suno

建议在 `RelayTask` / `RelayTaskFetch` 这条链上做独立适配：

- 提交类请求：记录提交参数、上游响应、公开 task id、上游 task id、计费上下文
- 查询类请求：记录查询参数、任务状态变化、结果摘要
- 二进制内容接口：仅记录元信息

重点：

- `task_id`
- `platform`
- `action`
- `origin_task_id`
- `public_task_id`
- `upstream_task_id`

## 6.3 Midjourney

建议在 `RelayMidjourney` 主入口和 `relay/mjproxy_handler.go` 相关函数加独立适配：

- submit
- fetch
- notify
- image-seed

说明：

- `/mj/image/:id` 属于二进制图片代理，只记录元信息
- `notify` 也属于“请求”，应纳入记录

## 6.4 统一上下文对象

建议在 `service/requestaudit` 中定义统一的采集上下文：

```text
Draft -> Attempts -> Finalize -> Persist
```

建议能力：

- `Begin(c, scope)`
- `CaptureRequest(...)`
- `AppendAttempt(...)`
- `CaptureResponse(...)`
- `CaptureError(...)`
- `Finalize(...)`
- `Persist(...)`

这样控制器只负责调用，不承载复杂拼装逻辑。

## 7. API 设计

## 7.1 详情查询

建议新增独立路由组，而不是塞进现有 `/api/log` 控制器：

```text
GET /api/request-audit/:request_id
```

权限规则：

- 管理员：可查任意 request_id
- 普通用户：仅当 `request_audits.user_id == current_user_id` 时可查

原因：

- 保持模块独立
- 避免把“列表日志”和“详情审计”控制器耦在一起

### 返回结构建议

```json
{
  "summary": {},
  "request": {},
  "response": {},
  "trace": {}
}
```

## 7.2 可选辅助接口

如后续需要，可扩展：

- `GET /api/request-audit/by-task/:task_id`
- `GET /api/request-audit/by-mj/:mj_id`

但第一版建议不做，优先通过现有日志页行数据里的标识直接发详情查询。

## 8. 前端接入设计

## 8.1 总体策略

新增一个可复用的“审计详情”组件：

```text
web/src/components/request-audit/*
```

建议形式：

- 详情抽屉优先
- 屏幕较小设备可回退全屏 Modal

内容分区建议：

1. 基础信息
2. 请求信息
3. 响应信息
4. 重试与渠道链路
5. 计费与 usage
6. 错误信息

## 8.2 使用日志页

在 `usage-logs` 中新增：

- 一列：`审计`
- 或在详情区增加 `查看审计` 按钮

推荐方案：

- 表格列中提供按钮
- 通过当前行 `request_id` 拉取详情

优点：

- 交互清晰
- 不污染现有展开内容

## 8.3 任务日志页

由于任务日志不一定出现在标准 `使用日志` 中，因此需要在 `task-logs` 页面也挂入口。

建议：

- 在表格中增加 `审计详情` 操作
- 通过 `request_id` 或任务关联标识查详情

## 8.4 绘图日志页

Midjourney 同理，需要单独接入。

建议：

- 在 `mj-logs` 页面增加 `审计详情` 操作
- 通过 `request_id` 或 `mj_id` 关联

## 8.5 前端显示原则

- 大文本使用代码块或只读 viewer
- JSON 支持折叠
- 文件元信息单独卡片展示
- 二进制响应明确标记“未保存原文”
- 对被脱敏字段展示脱敏后的值

## 9. 保留策略与配置

建议新增环境变量：

- `REQUEST_AUDIT_RETENTION_DAYS`
  - 默认 `30`

- `REQUEST_AUDIT_MAX_TEXT_BYTES`
  - 默认建议 `4194304`（4 MiB）
  - 用于极端情况下保护数据库体积

- `REQUEST_AUDIT_MAX_HEADER_BYTES`
  - 可选，保护头部异常膨胀

说明：

- 功能是否启用，不单独使用环境变量控制
- 启用逻辑由模式控制：仅在自用/演示模式下启用
- retention 清理建议复用现有后台 ticker 模式，仅 master 节点执行

## 10. 数据清理任务

建议新增后台清理任务：

- 启动位置：`main.go`
- 风格：参考现有订阅清理任务
- 行为：定时删除 `created_at < now - retention_days`

建议频率：

- 每 1 小时检查一次

## 11. 与现有日志的关系

### 11.1 保持不变的部分

- `model.Log` 仍是列表主数据源
- `controller/log.go` 接口仍照常工作
- 现有统计和筛选逻辑保持不变

### 11.2 需要补充的部分

- 确保所有新记录路径都能稳定生成 `request_id`
- `request_id` 应作为审计详情入口主键
- `task/mj` 页面需要额外暴露或补齐关联字段

## 12. 风险与限制

## 12.1 已知风险

1. 文本响应体可能非常大
   - 需要可配置上限与截断标记

2. 二进制识别需要做语义化裁剪
   - 不能简单保存原始 body

3. `mj/task` 与标准 relay 的链路不同
   - 需要分别适配，而不是只改 `controller/relay.go`

4. 前端入口不是单页统一
   - 需要复用详情组件，分别接入三类日志页

## 12.2 兼容性原则

- 不升级依赖
- 不改变现有日志页主查询接口
- 不改变现有消费/错误日志语义
- 不引入数据库方言特性

## 13. 推荐实施顺序

1. 后端独立模型与持久化
2. 标准 relay 审计采集
3. task / video / suno 审计采集
4. Midjourney 审计采集
5. 详情查询 API
6. 前端通用审计详情组件
7. 分别接入 `使用日志 / 任务日志 / 绘图日志`

## 14. 最终建议

第一版不做“独立审计列表页”，而是以“现有日志页 + 审计详情入口”的方式接入。

理由：

- 最小改动
- 用户认知成本低
- 与现有权限边界一致
- 不需要再造一套列表筛选系统
