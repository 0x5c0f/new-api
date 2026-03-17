# 请求审计功能说明

本目录用于存放 `request-audit-feat` 分支中“请求详细审计”功能的设计、实施计划、交接说明。

当前状态：该功能已经完成首轮落地，并且已接入现有日志页面。

## 1. 功能目标

- 仅在 `自用模式` 或 `演示模式` 下启用详细请求审计
- 尽量低入侵，不改现有消费日志主语义，不引入新依赖，不改上游核心配置体系
- 审计内容尽量完整，覆盖请求、响应、链路、模型映射、重试、关联日志
- 二进制内容不保存原文，只保存元信息
- 保留现有权限模型：普通用户看自己的，管理员看全量

## 2. 当前已实现内容

### 后端

- 新增独立审计表 `request_audits`
- 新增独立控制器与 API：
  - `/api/request-audit/:request_id`
  - `/api/request-audit/task/:task_id`
  - `/api/request-audit/mj/:mj_id`
- 接入标准 relay 审计
- 接入 playground 审计
- 接入 task / video / suno 任务链路审计
- 接入 Midjourney 审计
- 支持记录：
  - 请求头、查询参数、请求体
  - 响应头、响应体、SSE 聚合结果
  - 请求耗时、首包耗时、重试链路
  - 用户、令牌、分组、渠道、模型、任务 ID、MJ ID
  - 使用日志关联信息
  - 模型映射信息
- 增加 retention 清理任务

### 前端

- 在 `使用日志` 页增加“查看审计”按钮
- 在 `任务日志` 页增加“审计”列
- 在 `绘图日志` 页增加“请求审计”列
- 新增统一审计详情弹窗
- 支持查看：
  - 摘要信息
  - 请求
  - 响应
  - 链路
  - 原始概览
- 支持复制请求、响应、链路、`Request ID`
- 支持关联请求切换和筛选

## 3. 目录与文件

- [design.md](./design.md)：完整设计稿
- [implementation-plan.md](./implementation-plan.md)：实施计划
- [continue-prompt.md](./continue-prompt.md)：后续继续开发时可直接复用的提示词

关键实现文件：

- [service/request_audit.go](../../service/request_audit.go)
- [service/request_audit_test.go](../../service/request_audit_test.go)
- [model/request_audit.go](../../model/request_audit.go)
- [controller/request_audit.go](../../controller/request_audit.go)
- [web/src/components/request-audit/RequestAuditModal.jsx](../../web/src/components/request-audit/RequestAuditModal.jsx)
- [web/src/hooks/usage-logs/useUsageLogsData.jsx](../../web/src/hooks/usage-logs/useUsageLogsData.jsx)
- [web/src/hooks/task-logs/useTaskLogsData.js](../../web/src/hooks/task-logs/useTaskLogsData.js)
- [web/src/hooks/mj-logs/useMjLogsData.js](../../web/src/hooks/mj-logs/useMjLogsData.js)

## 4. 启用条件与配置

### 模式开关

- `自用模式`：启用详细请求审计
- `演示模式`：启用详细请求审计
- `对外运营模式`：不记录详细请求审计，前端不展示入口

### 环境变量

- `REQUEST_AUDIT_RETENTION_DAYS`
  - 默认值：`30`
  - 作用：控制审计记录保留天数
- `REQUEST_AUDIT_MAX_TEXT_BYTES`
  - 默认值：`4194304`（4 MiB）
  - 作用：限制单条文本请求/响应的保存体积，超过则截断

## 5. 审计内容说明

### 会保存的内容

- 请求方法、路径、模式、分组、用户、令牌、渠道
- 请求头、查询参数、请求体明文
- 响应头、响应体明文
- 流式响应聚合文本
- 状态码、成功标记、耗时、首包耗时、重试次数
- 任务 ID / MJ ID
- 请求模型、最终上游模型、模型映射关系
- 关联使用日志

### 不会保存原文的内容

- 图片
- 音频
- 视频
- PDF
- octet-stream
- multipart 上传文件原始二进制

这类内容只保留元信息，如大小、`content-type`、文件名等。

### 脱敏策略

- 认证头和常见密钥类字段会做脱敏处理
- 不直接保存完整 `Authorization`、`Cookie` 等敏感信息

## 6. 前端交互说明

### 使用日志

- 仅在审计启用模式下展示“审计”列
- 行内存在 `request_id` 时显示“查看审计”
- 审计详情接口请求成功后才打开弹窗

### 任务日志 / 绘图日志

- 通过 `task_id` / `mj_id` 拉取主审计记录
- 弹窗内可切换关联请求
- 默认优先显示更关键的主请求

### 弹窗

- 弹窗高度已限制在视口范围内，超出部分内部滚动
- 模型映射摘要会显示：
  - `请求模型 -> 上游模型`
- 若未发生映射，则显示“未发生映射”

## 7. 与上游兼容性约束

本功能的实现遵循以下原则：

- 不修改现有 `model.Log` 的主语义
- 不改现有日志权限模型
- 不引入新的第三方依赖
- 不升级前后端依赖版本
- 不改变现有 relay DTO 结构
- 不改变现有模式配置体系，只读取已有模式状态
- 不改现有上游品牌、标识、组织信息

当前入侵面控制在：

- 新增独立后端文件
- 在既有入口点增加极薄的审计挂点
- 在既有日志页增加列和详情弹窗

## 8. 已知边界

- 本轮已完整实跑 `自用模式`
- `对外运营模式` 本轮未切环境做浏览器实跑，但代码路径已保证不记录、不展示入口
- 公开图片代理类接口没有做用户态详细审计，以避免权限污染
- 任务 / Midjourney 的详情以主请求为主，关联请求通过弹窗切换查看
- 旧的历史审计记录如果当时未保存模型映射，现已通过“读取详情时结合使用日志回填”的方式修正

## 9. 建议的后续开发顺序

- 补齐审计列表筛选与导出能力
- 视需要增加管理员级审计检索页
- 若后续要支持更多异步任务平台，优先沿 `route_group + task_id/mj_id` 模式扩展
- 若后续要提升可观测性，优先补聚合检索和索引，不要先改现有日志表

## 10. 本地验证命令

后端测试：

```bash
go test ./service ./controller ./model ./router ./relay/...
```

本地镜像构建：

```bash
docker compose -f docker-compose.local.yml build
docker compose -f docker-compose.local.yml up -d
```

前端人工验证建议：

1. 登录后台
2. 打开 `使用日志`
3. 确认审计列与“查看审计”按钮可见
4. 打开一条映射模型请求的审计详情
5. 确认弹窗高度正常、模型映射显示正确
