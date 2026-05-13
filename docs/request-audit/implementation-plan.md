# 请求详细审计实施计划

## 1. 实施原则

- 默认最小改动
- 新功能独立成模块
- 优先复用现有 request_id、日志页、权限模型
- 不改现有消费日志主语义
- 所有数据库改动同时兼容 SQLite / MySQL / PostgreSQL

## 2. 建议目录与文件

### 后端

- `model/request_audit.go`
- `service/requestaudit/collector.go`
- `service/requestaudit/sanitizer.go`
- `service/requestaudit/persist.go`
- `service/requestaudit/cleanup_task.go`
- `controller/request_audit.go`

### 前端

- `web/src/components/request-audit/AuditDetailDrawer.jsx`
- `web/src/components/request-audit/AuditSection.jsx`
- `web/src/components/request-audit/renderers/*`

### 接入点

- `controller/relay.go`
- `relay/relay_task.go`
- `relay/mjproxy_handler.go`
- `router/api-router.go`
- `main.go`
- `web/src/components/table/usage-logs/*`
- `web/src/components/table/task-logs/*`
- `web/src/components/table/mj-logs/*`

## 3. 后端阶段拆分

### 阶段 A：数据模型与清理能力

- 新增 `RequestAudit` 模型
- 新增表初始化与迁移逻辑
- 新增 retention 清理函数
- 新增后台清理任务
- 新增环境变量读取

验收标准：

- 启动后可自动建表
- 三种数据库均能通过迁移
- retention 可按天数清理历史记录

### 阶段 B：标准 relay 审计采集

- 在 `controller/relay.go` 接入审计生命周期
- 采集 request headers / query / body
- 采集 retry attempts
- 采集 response 聚合结果
- 采集 billing / usage / error / conversion chain

验收标准：

- chat/completions、responses、embeddings、images、audio、realtime 可生成记录
- 仅在自用/演示模式下生成
- 普通运营模式下不生成

### 阶段 C：task / video / suno 审计采集

- 在 `RelayTask` / `RelayTaskFetch` 接入
- 记录 task submit / fetch / remix
- 记录公开 task id 与上游 task id
- 二进制结果仅记录元信息

验收标准：

- submit / fetch 都可查到详情
- 失败与成功都可追溯

### 阶段 D：Midjourney 审计采集

- 在 submit / fetch / notify / image-seed 接入
- `/mj/image/:id` 只记元信息

验收标准：

- mj 主要操作都能生成审计
- 图片代理不落二进制正文

### 阶段 E：详情查询 API

- 新增 `GET /api/request-audit/:request_id`
- 完成权限校验
- 支持管理员看全量、普通用户看自己

验收标准：

- 普通用户无法读取他人的 request_id
- 管理员可以跨用户查看

## 4. 前端阶段拆分

### 阶段 F：通用详情组件

- 新增审计详情抽屉/弹窗
- 支持基础信息、请求、响应、重试、计费、错误多个区块
- 大文本支持折叠/复制
- JSON 支持格式化

验收标准：

- 可以复用，不依赖单一日志页

### 阶段 G：接入使用日志页

- 在 `usage-logs` 增加 `查看审计` 操作
- 通过 `request_id` 拉取详情

验收标准：

- 普通用户能看自己的标准 relay 审计
- 管理员能看全量标准 relay 审计

### 阶段 H：接入任务日志页

- 在 `task-logs` 增加 `查看审计` 操作
- 处理 task 标识与 request_id 的映射

验收标准：

- task submit/fetch 可以从任务页进入审计详情

### 阶段 I：接入绘图日志页

- 在 `mj-logs` 增加 `查看审计` 操作
- 处理 `mj_id` 与 request_id 的映射

验收标准：

- Midjourney 相关请求可以从绘图日志页进入审计详情

## 5. 关键实现选择

### 选择 1：审计详情不进入现有 `record.other`

原因：

- `record.other` 适合轻量信息
- 大文本塞入会让现有日志接口和前端都变重

### 选择 2：详情用独立 API，而不是扩展 `/api/log/self`

原因：

- 模块边界更清楚
- 列表查询和详情读取解耦

### 选择 3：前端不做新列表页

原因：

- 现有三类日志页已经是天然入口
- 直接加详情查看最省改动

## 6. 建议优先级

建议开发顺序：

1. A
2. B
3. E
4. F
5. G
6. C
7. H
8. D
9. I

说明：

- 先把标准 relay 跑通，能最快验证设计正确性
- task 和 mj 后补，可以减少第一轮联调面

## 7. 验收清单

### 模式开关

- 自用模式下有详细审计
- 演示模式下有详细审计
- 对外运营模式下无详细审计

### 权限

- 普通用户只能查看自己的审计详情
- 管理员可以查看所有审计详情

### 内容

- 文本请求明文可见
- 文本响应明文可见
- 二进制仅元信息
- 关键 headers / keys 脱敏

### 页面

- `使用日志` 可查看审计详情
- `任务日志` 可查看审计详情
- `绘图日志` 可查看审计详情

### 稳定性

- 不影响现有日志列表性能
- 不影响现有消费统计
- retention 生效

## 8. 当前阶段建议

下一步开发建议直接从“后端模型 + 标准 relay 接入”开始。

原因：

- 风险最可控
- 最容易做端到端验证
- 设计中的大部分机制都能在这条链先跑通
