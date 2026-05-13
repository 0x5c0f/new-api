# 后续开发可复用提示词

## 审计功能继续开发

你当前接手的是 `new-api` 仓库中 `feat-custom/request-audit-file-storage` 分支上的“请求详细审计”功能。完整说明见 [README.md](./README.md)，设计文件见 [file-storage-design.md](./file-storage-design.md)，合并指南见 [upstream-merge-guide.md](./upstream-merge-guide.md)。

## 必须遵守的规范

- **首先确认 CLAUDE.md 中包含 Rule 8（低耦合补丁原则）**。如果不存在，需要从 [rule8-low-coupling.md](./rule8-low-coupling.md) 复制到项目根目录 `CLAUDE.md` 的 `## Rules` 节点下。此规则是本次开发的核心指导原则，所有改动都在尽量不影响上游仓库的前提下进行。
- 阅读并遵守根目录 `CLAUDE.md`（尤其是 Rule 1 JSON 包、Rule 2 数据库兼容）
- 后端 JSON 编解码统一走 `common/json.go`
- 数据库必须兼容 SQLite / MySQL / PostgreSQL
- 不要升级依赖，不要换框架，不要改项目和组织标识
- 继续遵循"最小改动、低耦合、沿现有边界走"

## 当前已完成

- 独立审计表 `request_audits`（含文件存储路径字段）
- 标准 relay / playground / task / Midjourney / video 审计接入
- 审计查询 API（3 个端点）
- Payload 文件存储（DB-first 同步写入 + gopool.Go 异步写文件）
- 组合 JSON 文件格式 + 按 key 提取的读取回退
- DB payload 字段保留用于旧数据兼容
- 使用日志 / 任务日志 / 绘图日志三处经典前端入口
- 审计详情弹窗（请求/响应/链路/原始概览 + 下载审计日志按钮）
- 请求/响应/链路/Request ID 复制功能
- 二进制只存元信息
- 敏感字段脱敏
- retention 清理任务（含文件清理）
- 旧数据迁移（启动时异步执行）
- 模型映射显示与回填
- 模式开关验证（对外运营模式下不记录、不展示）

## 待优化/扩展方向

- **默认前端**（React 19 + Base UI + TanStack Table）审计支持
- 审计记录独立列表页 + 筛选检索
- 管理员级审计搜索
- 更细的 task / mj 审计分类展示

## 关键约束

- 详细审计不复用现有 `model.Log` 作为主存储
- 审计详情默认沿用"普通用户看自己，管理员看全量"权限模型
- 对外运营模式下不应记录审计，也不应展示审计入口
- Payload 文件路径使用 `request_payload_path` 作为统一入口（三个 payload 在同一文件）
- 异步文件写入有极短窗口期（毫秒级）数据不可用，这是已知权衡

## 关键文件映射

| 文件 | 职责 |
|------|------|
| `common/request_audit_storage.go` | 文件 I/O |
| `model/request_audit.go` | DB 模型 + 迁移/查询 |
| `service/request_audit.go` | 审计生命周期 + 写入/清理/迁移 |
| `controller/request_audit.go` | API 端点 + 读取逻辑 |
| `controller/relay.go` | 审计钩子挂点 |
| `router/api-router.go` | 路由注册 |
| `main.go` | 启动初始化 |
| `web/classic/src/components/request-audit/RequestAuditModal.jsx` | 审计详情弹窗 |
| `web/classic/src/hooks/*-logs/use*Data.*` | 审计列状态管理 |
| `web/classic/src/components/table/*-logs/*` | 审计列 + 弹窗集成 |

## 已避免的问题（不可回退）

- 不要把审计列显示逻辑写成"模式状态未加载时直接按外部模式处理"
- 不要再用本地存储 key 硬切缓存版本
- 打开审计详情时，不要先开空弹窗再请求接口
- 模型映射不能只依赖 relay 当场元信息，需结合使用日志回填
- 弹窗高度必须限制在视口内，内容区内部滚动
- Payload 文件写入时不要用三个独立文件（统一组合 JSON）
- 三个 tab 读取时必须按 key 分别提取（不要全量返回给单个 tab）

## 本地验证命令

```bash
# 后端
go test ./common/... ./model/... ./service/... ./controller/... ./router/...

# 前端构建
cd web/classic && bun install && bun run build

# Docker
docker compose -f docker-compose.local.yml build
docker compose -f docker-compose.local.yml up -d
```
