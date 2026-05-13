# 上游仓库同步与合并注意事项

## 分支策略

```
main                              ← 上游同步（只读，不在本地改动）
  └── feat-custom/release         ← 定制功能集成分支
       └── feat-custom/request-audit-file-storage  ← 当前分支
```

## 与上游同步时的合并风险矩阵

### 无冲突区域（自动合并）

以下文件是**完全新增**的，上游不包含，合并时不会冲突：

| 文件 | 类型 |
|------|------|
| `common/request_audit_storage.go` | 新增 |
| `model/request_audit.go` | 新增 |
| `service/request_audit.go` | 新增 |
| `controller/request_audit.go` | 新增 |
| `web/classic/src/components/request-audit/RequestAuditModal.jsx` | 新增 |

### 需手动检查的文件（轻度变更）

这些文件在上游也有，但我们的改动非常小（仅函数调用钩子），冲突风险低：

| 文件 | 变更内容 | 冲突风险 |
|------|----------|----------|
| `model/main.go` | `migrateLOGDB()` 中添加 `&RequestAudit{}` AutoMigrate | 低 — 只有一行新增 |
| `main.go` | `InitLogDB()` 后添加表迁移 + 清理任务 + 启动迁移 | 低 — 新增 9 行 |
| `router/api-router.go` | 新增 3 条审计路由 | 低 — 新增 5 行 |

### 需要仔细检查的文件（中度变更）

| 文件 | 变更内容 | 冲突风险 |
|------|----------|----------|
| `controller/relay.go` | 在 `Relay()` / `RelayMidjourney()` / `RelayTaskFetch()` / `RelayTask()` 中添加审计生命周期钩子 | 中 — 如果上游重构 relay 函数签名，需要调整钩子位置 |

### 前端文件（中度变更）

| 文件 | 变更内容 | 冲突风险 |
|------|----------|----------|
| `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx` | 审计列可见性管理 + API 调用 | 中 |
| `web/classic/src/hooks/task-logs/useTaskLogsData.js` | 审计列可见性管理 | 中 |
| `web/classic/src/hooks/mj-logs/useMjLogsData.js` | 审计列可见性管理 + API 调用 | 中 |
| 6 个 ColumnDefs/Table 文件 | 审计列 + 审计弹窗集成 | 低-中 |

## 合并操作步骤

### 1. 同步上游 main

```bash
git fetch upstream main
git checkout feat-custom/release
git merge upstream/main --no-commit
# 解决冲突后提交
```

### 2. 将审计分支 rebase 到更新后的 release

```bash
git checkout feat-custom/request-audit-file-storage
git rebase feat-custom/release
```

### 3. 检查关键文件冲突

优先检查以下文件（按风险从高到低）：

1. `controller/relay.go` — 确认审计钩子位置正确
   - 搜索 `BeginRequestAudit` / `CaptureRequestAuditRelayInfo` / `FinishRequestAudit` / `AppendRequestAuditAttempt`
2. `main.go` — 确认表迁移和清理任务调用仍然在 `InitLogDB()` 之后
3. 前端 hooks — 确认审计列可见性逻辑完整

### 4. 验证

```bash
go build ./...
go test ./common/... ./model/... ./service/... ./controller/...
docker compose -f docker-compose.local.yml build
docker compose -f docker-compose.local.yml up -d
```

## 关键检查清单

合并后必须确认以下功能正常：

- [ ] 自用模式下 `使用日志` 存在"查看审计"列和按钮
- [ ] 点击"查看审计"打开详情弹窗
- [ ] 弹窗中请求/响应/链路/原始概览四个 tab 内容正确
- [ ] 模型映射显示正确
- [ ] 对外运营模式下无审计列、无记录
- [ ] 后端测试全部通过
- [ ] Payload 文件正确生成在 `{log-dir}/request-audit/{YYYY-MM-DD}/{request_id}/payload.json`
- [ ] 表 `request_audits` 存在且结构正确（6 个路径/大小字段 + 3 个 payload 字段）

## 上游 relay 重构的应对

如果上游重构了 `controller/relay.go` 的函数签名或结构，需要确保审计钩子仍然在正确的位置：

```
在每个 relay 入口函数中：
  1. 函数开头：auditState := service.BeginRequestAudit(c, "route_group")
  2. defer 块：service.FinishRequestAudit(c)
  3. GenRelayInfo 之后：service.CaptureRequestAuditRelayInfo(c, relayInfo)
  4. 错误发生时：service.CaptureRequestAuditError(c, err)
  5. 重试时：service.AppendRequestAuditAttempt(c, ...)
```
