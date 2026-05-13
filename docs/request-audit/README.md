# 请求审计功能说明

本目录包含 `feat-custom/request-audit-file-storage` 分支中"请求详细审计"功能的完整设计与实现文档。

**当前状态：** 审计功能已完整落地（基础功能 + 文件存储优化），已接入后端 relay 全链路和经典前端三类日志页。对外运营模式验证通过。

## 1. 功能目标

- 仅在 `自用模式` 或 `演示模式` 下启用详细请求审计
- 尽量低入侵，不改现有消费日志主语义，不引入新依赖，不改上游核心配置体系
- 审计内容尽量完整，覆盖请求、响应、链路、模型映射、重试、关联日志
- 二进制内容不保存原文，只保存元信息
- 保留现有权限模型：普通用户看自己的，管理员看全量
- **Payload 文件存储**：请求/响应/链路 payload 存入文件系统，DB 仅存路径指针，避免数据库膨胀

## 2. 目录结构

| 文件 | 说明 |
|------|------|
| [README.md](./README.md) | 本文件，功能总览 |
| [design.md](./design.md) | 完整设计稿（架构、数据模型、敏感信息策略） |
| [implementation-plan.md](./implementation-plan.md) | 分阶段实施计划（A-I 阶段） |
| [file-storage-design.md](./file-storage-design.md) | 文件存储优化设计决策 |
| [upstream-merge-guide.md](./upstream-merge-guide.md) | 上游同步与合并注意事项 |
| [rule8-low-coupling.md](./rule8-low-coupling.md) | 低耦合补丁原则（Rule 8CLAUDE.md 可复制片段） |
| [continue-prompt.md](./continue-prompt.md) | 后续开发可复用的提示词 |

## 3. 当前已实现内容

### 后端

| 文件 | 职责 |
|------|------|
| `common/request_audit_storage.go` | 文件存储基础设施（读/写/删除 payload 文件） |
| `model/request_audit.go` | 数据模型 + 路径/大小字段 + 迁移/清理辅助函数 |
| `service/request_audit.go` | 审计生命周期管理 + DB-first 写入 + 异步文件写入 + 迁移 + 保留清理 |
| `controller/request_audit.go` | 3 个查询 API + 文件/DB 回退读取逻辑 |
| `controller/relay.go` | 审计钩子挂点（Relay / RelayMidjourney / RelayTaskFetch / RelayTask） |
| `router/api-router.go` | 路由注册（3 条审计 API） |
| `main.go` | 启动时表迁移 + 清理任务 + 旧数据迁移 |

**API 端点：**
- `GET /api/request-audit/:request_id` — 按请求 ID 查询审计详情
- `GET /api/request-audit/task/:task_id` — 按任务 ID 查询审计详情（含关联记录）
- `GET /api/request-audit/mj/:mj_id` — 按 MJ ID 查询审计详情（含关联记录）

### 前端（经典前端 — Semi Design）

| 文件 | 职责 |
|------|------|
| `web/classic/src/components/request-audit/RequestAuditModal.jsx` | 审计详情弹窗（摘要、请求、响应、链路、原始概览） |
| `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx` | 使用日志审计列状态与可见性管理 |
| `web/classic/src/hooks/task-logs/useTaskLogsData.js` | 任务日志审计列状态 |
| `web/classic/src/hooks/mj-logs/useMjLogsData.js` | 绘图日志审计列状态 |
| `web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx` | 使用日志审计列定义 |
| `web/classic/src/components/table/usage-logs/UsageLogsTable.jsx` | 使用日志审计弹窗集成 |
| `web/classic/src/components/table/task-logs/TaskLogsColumnDefs.jsx` | 任务日志审计列定义 |
| `web/classic/src/components/table/task-logs/TaskLogsTable.jsx` | 任务日志审计弹窗集成 |
| `web/classic/src/components/table/mj-logs/MjLogsColumnDefs.jsx` | 绘图日志审计列定义 |
| `web/classic/src/components/table/mj-logs/MjLogsTable.jsx` | 绘图日志审计弹窗集成 |

### 前端功能

- 使用日志 / 任务日志 / 绘图日志 三类日志页的"查看审计"操作
- 审计详情弹窗（摘要信息、请求、响应、链路、原始概览四个 tab）
- 复制请求/响应/链路/Request ID
- **下载完整审计日志**（导出为 JSON 文件）
- 关联请求切换与按类别筛选
- 模型映射显示（`请求模型 -> 上游模型` / `未发生映射`）
- 弹窗高度限制在视口范围内，内部可滚动

## 4. 模式开关

| 模式 | 审计记录 | 前端入口 |
|------|----------|----------|
| 自用模式 (`SelfUseModeEnabled = true`) | ✅ 记录 | ✅ 显示 |
| 演示模式 (`DemoSiteEnabled = true`) | ✅ 记录 | ✅ 显示 |
| 对外运营模式（两者都为 false） | ❌ 不记录 | ❌ 不显示 |

## 5. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `REQUEST_AUDIT_RETENTION_DAYS` | `30` | 审计记录保留天数 |
| `REQUEST_AUDIT_MAX_TEXT_BYTES` | `4194304` (4 MiB) | 单条文本请求/响应最大保存体积 |
| `REQUEST_AUDIT_FILE_DIR` | `*LogDir` 的值 | Payload 文件存储根目录；默认在 `{log-dir}/request-audit/` |

## 6. 文件存储设计要点

1. **组合 JSON 格式**：payload 文件为单一 JSON：`{"request": {...}, "response": {...}, "trace": {...}}`
2. **日期目录组织**：`{root}/request-audit/{YYYY-MM-DD}/{request_id}/payload.json`
3. **DB-first + 异步文件写入**：DB 记录先同步写入（payload 字段置空），`gopool.Go()` 异步写文件后更新路径
4. **读取回退**：文件路径非空 → 读文件；路径为空 → 读 DB payload 字段（旧数据兼容）
5. **写入失败处理**：记录错误日志，丢弃，路径保持为空
6. **保留清理**：按日期目录批量删除（`os.RemoveAll`）

## 7. 与上游兼容性约束

- 不修改现有 `model.Log` 的主语义
- 不改现有日志权限模型
- 不引入新的第三方依赖
- 不升级前后端依赖版本
- 不改变现有 relay DTO 结构
- 不改变现有模式配置体系，只读取已有模式状态

## 8. 待完成工作

- **默认前端**（React 19 + Base UI + TanStack Table）审计弹窗和日志列 — 需单独开发
- 审计记录独立列表页
- 审计记录筛选与检索

## 9. 本地验证

```bash
# 后端
go test ./common/... ./model/... ./service/... ./controller/... ./router/...

# Docker
docker compose -f docker-compose.local.yml build
docker compose -f docker-compose.local.yml up -d

# 功能验证
# 1. 自用模式下使用日志存在"查看审计"列
# 2. 点击打开审计详情弹窗
# 3. 弹窗高度不撑满全屏
# 4. 模型映射显示正确
# 5. 对外运营模式下无审计列、无记录
# 6. Payload 文件生成在 {log-dir}/request-audit/{YYYY-MM-DD}/{request_id}/payload.json
```
