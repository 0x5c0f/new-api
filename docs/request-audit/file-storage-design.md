# 请求审计 — Payload 文件存储设计

## 背景

原始审计实现将完整的请求/响应/链路 payload 以 TEXT/MEDIUMTEXT 列存入 `request_audits` 表，单行最坏情况可达 ~48MB，导致 DB 膨胀。文件存储方案将 payload 移到文件系统，DB 仅存路径指针。

## 核心设计决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 文件路径 | `{root}/request-audit/{YYYY-MM-DD}/{request_id}/payload.json` | 按日期分区，便于清理；按 request_id 隔离 |
| 根目录 | `REQUEST_AUDIT_FILE_DIR` 环境变量，fallback 到 `*common.LogDir` | 灵活配置，默认与日志目录一致 |
| 文件格式 | 单一 JSON：`{"request": ..., "response": ..., "trace": ...}` | 一次 I/O 完成读写，减少文件碎片 |
| DB 新增字段 | 6 列：3 个路径 (`varchar(512)`) + 3 个大小 (`bigint`) | 保留原始 payload 字段用于旧数据兼容 |
| 写入时机 | DB 先同步写入（payload 置空），`gopool.Go()` 异步写文件 | 不影响 relay 链路延迟 |
| 写入失败 | 记录错误日志，丢弃，路径保持为空 | 不阻塞业务，不重试 |
| 读取回退 | 路径非空 → 读文件并按 key 提取；路径为空 → 读 DB payload | 无缝兼容旧数据和新数据 |
| 旧数据迁移 | 启动时 `gopool.Go()` 异步批量迁移（每批 100 条） | 逐步迁移，不阻塞启动 |
| 保留清理 | 删除 DB 行前收集路径 → 按日期目录 `os.RemoveAll` | 原子清理，高效 |

## 文件路径规则

```
{REQUEST_AUDIT_FILE_DIR || *LogDir}/request-audit/{YYYY-MM-DD}/{request_id}/payload.json
```

示例：
```
./logs/request-audit/2026-05-13/202605130127453741328948268d9d6ELsuNrBp/payload.json
```

## 文件内容结构

```json
{
  "request": {
    "headers": {...},
    "query": {...},
    "remote_ip": "...",
    "content_type": "application/json",
    "body_kind": "json",
    "body_size": 1234,
    "body_text": "...",
    "body_json": {...}
  },
  "response": {
    "headers": {...},
    "content_type": "application/json",
    "body_kind": "json",
    "body_size": 5678,
    "body_text": "...",
    "body_json": {...}
  },
  "trace": {
    "request_conversion": {...},
    "model_resolution": {...},
    "billing": {...},
    "attempts": [...],
    "linked_logs": [...]
  }
}
```

## DB 字段变更

### 新增字段 (`model/request_audit.go`)

```go
RequestPayloadPath  string `gorm:"type:varchar(512);default:''"`
ResponsePayloadPath string `gorm:"type:varchar(512);default:''"`
TracePayloadPath    string `gorm:"type:varchar(512);default:''"`
RequestPayloadSize  int64  `gorm:"bigint;default:0"`
ResponsePayloadSize int64  `gorm:"bigint;default:0"`
TracePayloadSize    int64  `gorm:"bigint;default:0"`
```

### 保留字段（向后兼容）

```go
RequestPayload  RequestAuditPayload `json:"request_payload"`
ResponsePayload RequestAuditPayload `json:"response_payload"`
TracePayload    RequestAuditPayload `json:"trace_payload"`
```

注意：三个 payload 存在同一个文件中，实际只有 `request_payload_path` 被填充，`response_payload_path` 和 `trace_payload_path` 保持为空。

## 写入流程

```
FinishRequestAudit()
  ├── captureRequestAuditResponse()  // 捕获响应数据
  ├── attachLinkedLogs()             // 关联消费日志
  ├── syncRequestAuditRelayInfo()    // 同步 relay 信息
  ├── DB payload 字段置空
  ├── model.UpsertRequestAudit()     // 同步写入 DB（轻量）
  └── gopool.Go(func() {            // 异步写文件
        ├── common.WriteRequestAuditPayload(requestID, combined)
        └── model.UpdateRequestAuditPayloadPaths(auditID, path, size, "", 0, "", 0)
      })
```

## 读取流程

```
buildRequestAuditResponse(audit)
  ├── filePath = audit.RequestPayloadPath  // 统一路径
  ├── readAuditPayload(filePath, audit.RequestPayload, "request")   // 提取 "request" key
  ├── readAuditPayload(filePath, audit.ResponsePayload, "response") // 提取 "response" key
  └── readAuditPayload(filePath, audit.TracePayload, "trace")       // 提取 "trace" key

readAuditPayload(path, dbPayload, key):
  if path != "":
    data = ReadRequestAuditPayload(path)    // 读文件
    combined = Unmarshal(data)
    return combined[key]                     // 按 key 提取
  else:
    return parseAuditPayload(dbPayload)      // 旧数据回退
```

## 已知权衡

- **异步写入窗口**：DB 写入后、文件写入完成前，路径为空，此时查询 API 返回空 payload。窗口时间为毫秒级，实际使用中几乎不会遇到。
- **单文件模式**：所有三个 payload 在一个文件中。单个 payload 损坏不影响其他（整个文件损坏才会全丢失）。
- **文件系统依赖**：需要文件系统可写且空间充足。写入失败时只记日志，不阻塞业务。
