# Rule 8: 自定义功能开发 — 低耦合补丁原则

当开发自定义功能（上游项目不存在的功能）时，必须遵循低耦合补丁原则，以最小化与上游同步时的合并冲突。

**核心原则**：自定义功能是叠加在上游项目之上的"补丁"。必须对现有代码产生最小影响，确保上游更新可以零冲突或近零冲突合并。

## 强制约束

1. **独立模块优先**：新逻辑必须放在新文件中（如 `service/request_audit.go`、`model/request_audit.go`）。除非绝对必要，不要将逻辑硬塞进现有文件。

2. **仅最薄钩子点**：触碰现有文件时，添加尽可能薄的钩子（如在明确定义的入口点添加单个函数调用）。不要重构、重构或"改进"现有代码。

3. **不变更上游结构**：不要修改现有模型 schema、DTO 结构、路由组、中间件链或控制器逻辑（超出最小钩子范围）。

4. **不引入新依赖**：不要引入新的第三方库。仅使用项目已导入的内容。

5. **不升级版本**：不要在自定义功能中升级 Go、npm 或任何依赖版本。

6. **保持现有语义**：不要更改现有 API、日志表或配置系统的含义、行为或契约。自定义功能扩展，从不替换。

7. **数据库作为指针存储**：当自定义功能需要存储大 payload 时，将其存入文件系统，仅在数据库中保留路径指针。这减少 DB 压力并保持现有表结构清晰。

## 如何应用

写任何代码之前，问自己："如果明天上游改了同一个文件，合并难度有多大？"——如果答案是"困难"，就降低耦合。

## 如何在新的开发会话中启用此规则

**此规则必须加入到项目根目录 `CLAUDE.md` 的 `## Rules` 节点下，作为 `### Rule 8`。**

如果 CLAUDE.md 中尚不存在此规则，请在开始开发前添加。以下是可直接复制到 CLAUDE.md 的 Markdown 片段：

```markdown
### Rule 8: Custom Feature Development — Low Coupling Patch Principle

When developing custom features (additions that do NOT exist in the upstream project), you MUST follow the low-coupling patch principle to minimize merge conflicts when syncing with upstream.

**Core principle**: Custom features are "patches" layered on top of the upstream project. They must have minimal impact on existing code, ensuring that upstream updates can be merged with zero or near-zero conflicts.

**Mandatory constraints:**

1. **Independent modules first**: New logic MUST live in new files (e.g., `service/request_audit.go`, `model/request_audit.go`). Do NOT hard-push logic into existing files unless absolutely necessary.

2. **Thin hook points only**: When touching existing files, add the thinnest possible hook (e.g., a single function call at a well-defined entry point). Do NOT refactor, restructure, or "improve" existing code.

3. **No upstream structure changes**: Do NOT modify existing model schemas, DTO structures, router groups, middleware chains, or controller logic beyond the minimal hook.

4. **No new dependencies**: Do NOT introduce new third-party libraries. Use only what the project already imports.

5. **No version upgrades**: Do NOT upgrade Go, npm, or any dependency version as part of a custom feature.

6. **Preserve existing semantics**: Do NOT change the meaning, behavior, or contract of existing APIs, log tables, or configuration systems. Custom features extend, never replace.

7. **Database as pointer store**: When custom features need to store large payloads, store them in the filesystem and keep only path pointers in the database. This reduces DB pressure and keeps the existing table schema clean.

**How to apply**: Before writing any code, ask: "If upstream changes this file tomorrow, how hard will it be to merge?" If the answer is "hard," reduce the coupling.
```
