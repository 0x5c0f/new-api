# 请求审计功能继续开发提示词

你当前接手的是 `new-api` 仓库中 `request-audit-feat` 分支上的“请求详细审计”功能。

请严格遵守以下上下文与约束，再继续开发：

## 1. 基本背景

- 仓库：`new-api`
- 当前分支：`request-audit-feat`
- `main` 仅用于同步上游，不在本地直接改动
- 当前功能目标：
  - 仅在 `自用模式` 或 `演示模式` 下记录详细请求审计
  - 尽量低入侵，降低与上游同步后的合并冲突
  - 前后端都已接入到现有日志体系

## 2. 必须遵守的项目规范

- 阅读并遵守根目录 `AGENTS.md`
- 后端 JSON 编解码统一走 `common/json.go`
- 数据库必须兼容 SQLite / MySQL / PostgreSQL
- 不要升级依赖，不要换框架
- 不要改项目和组织标识
- 继续保持“最小改动、低耦合、沿现有边界走”

## 3. 当前功能现状

已完成：

- 独立审计表 `request_audits`
- 标准 relay / playground / task / Midjourney / video 相关审计接入
- 审计查询 API
- 使用日志 / 任务日志 / 绘图日志 三处前端入口
- 审计详情弹窗
- 请求/响应/链路保存
- 二进制只存元信息
- retention 清理任务
- 模型映射显示
- 历史错误缓存导致的审计列隐藏问题修复

关键约束：

- 详细审计不复用现有 `model.Log` 作为主存储
- 审计详情默认依然沿用“普通用户看自己，管理员看全量”
- 对外运营模式下不应记录审计，也不应展示审计入口

## 4. 关键文件

后端：

- `service/request_audit.go`
- `service/request_audit_test.go`
- `model/request_audit.go`
- `controller/request_audit.go`
- `controller/relay.go`
- `controller/video_proxy.go`
- `relay/mjproxy_handler.go`
- `router/api-router.go`
- `main.go`

前端：

- `web/src/components/request-audit/RequestAuditModal.jsx`
- `web/src/hooks/usage-logs/useUsageLogsData.jsx`
- `web/src/hooks/task-logs/useTaskLogsData.js`
- `web/src/hooks/mj-logs/useMjLogsData.js`
- `web/src/components/table/usage-logs/*`
- `web/src/components/table/task-logs/*`
- `web/src/components/table/mj-logs/*`

文档：

- `docs/request-audit/README.md`
- `docs/request-audit/design.md`
- `docs/request-audit/implementation-plan.md`

## 5. 已踩过的问题，不要再回退

- 不要把审计列显示逻辑写成“模式状态未加载时直接按外部模式处理”，否则会把本地缓存错误写成 `audit=false`
- 不要再用新的本地存储 key 硬切缓存版本，优先兼容原有缓存机制
- 打开审计详情时，不要先开空弹窗再请求接口
- 模型映射不能只依赖 relay 当场元信息，详情读取时要允许结合 `request_id` 对应的使用日志做兜底回填
- 弹窗高度必须限制在视口内，内容区内部滚动

## 6. 继续开发时的原则

- 优先新增独立模块，不把逻辑硬塞进旧日志表
- 优先复用现有日志页和权限模型
- 新增能力先从 `route_group + request_id/task_id/mj_id` 维度扩展
- 如果要增强检索，优先审计详情和筛选能力，不要先重构整套日志架构
- 任何改动后都要跑完整验证

## 7. 最低验证要求

至少执行：

```bash
go test ./service ./controller ./model ./router ./relay/...
docker compose -f docker-compose.local.yml build
docker compose -f docker-compose.local.yml up -d
```

至少人工确认：

1. 自用模式下 `使用日志` 存在“查看审计”
2. 审计详情可正常打开
3. 弹窗高度不撑满全屏
4. 映射模型能显示 `请求模型 -> 上游模型`
5. `任务日志` / `绘图日志` 的审计列可见

如果本轮动到了模式判断或前端列缓存：

6. 确认旧本地缓存不会把审计列永久隐藏

## 8. 回答与交付风格

- 不编不存在的项目背景
- 明确区分：
  - 仓库已确认
  - 合理推断
  - 待确认项
- 默认最小改动
- 如果与现有架构冲突，先指出冲突点，再给方案

## 9. 如果要继续扩展，建议优先方向

- 审计详情导出 JSON
- 审计记录筛选与检索
- 审计列表页
- 管理员级审计搜索
- 更细的 task / mj 审计分类展示
