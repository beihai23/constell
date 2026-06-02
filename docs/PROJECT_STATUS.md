# Constell — 项目状态

> **任何新会话的开场白：** 读取此文件了解项目全貌和当前状态，然后按指引工作。

## 项目简介

Constell（星座）是一个开源社群型 IM 系统，类似 Discord。后端 Go 微服务，前端 Web，SDK（Go/JS/KMP）。

## 核心文档

| 文档 | 用途 | 大小 |
|------|------|------|
| `docs/superpowers/specs/2026-05-29-constell-architecture-design.md` | **架构 spec — 所有实施细节的唯一上下文源** | 699 行 |
| `docs/superpowers/specs/2026-06-02-constell-governance-design.md` | **治理 spec — 服务发现/配置/健康检查/可观测性设计** | ~650 行 |
| `docs/superpowers/specs/2026-06-02-constell-ws-gateway-design.md` | **WS Gateway spec — 连接管理/协议/路由/扇出设计** | ~490 行 |
| `docs/superpowers/specs/2026-06-02-constell-plan4-file-search-notify-design.md` | **Plan 4 spec — File/Search/Notify 设计** | ~750 行 |
| `docs/superpowers/plans/2026-05-30-constell-plans-overview.md` | 阶段计划概要（需更新为 6 个阶段） | 142 行 |
| `docs/superpowers/plans/2026-05-30-plan1-foundation-core.md` | Plan 1 详细实施计划（22 个 Task） | ~10,700 行 |
| `docs/superpowers/plans/2026-06-02-plan2-governance.md` | Plan 2 详细实施计划（16 个 Task） | ~2,600 行 |
| `docs/superpowers/plans/2026-05-30-plan2-ws-gateway.md` | Plan 3 详细实施计划（17 个 Task） | ~5,500 行 |
| 本文件 | 项目状态 + 工作规则 | — |

## 阶段进度

| 阶段 | 计划文件 | 状态 | 说明 |
|------|----------|------|------|
| Plan 1: 基础设施 + 核心服务 | `plans/2026-05-30-plan1-foundation-core.md` | ✅ 已完成 | 全部 22 Tasks 完成，集成测试 + Docker Compose 全服务配置就绪 |
| Plan 2: 服务治理 | `plans/2026-06-02-plan2-governance.md` | ✅ 已完成 | 全部 16 Tasks 完成：服务发现 + 配置管理 + 健康检查 + 可观测性 (OTel/OpenObserve) |
| Plan 3: WS Gateway | `plans/2026-05-30-plan2-ws-gateway.md` | ✅ 已完成 | 全部 17 Tasks 已在 Plan 1 + Plan 2 阶段实现，含 proto/protocol/auth/connmgr/registry/router/push/heartbeat/server，测试通过 |
| Plan 4: File + Search + Notify | 概要在 plans-overview 中 | 📋 计划就绪 | |
| Plan 5: Web 客户端 | 概要在 plans-overview 中 | ⏳ 待规划 | |
| Plan 6: SDK | 概要在 plans-overview 中 | ⏳ 待规划 | |

## 推进节奏

**规则：当前阶段实现时，写好下一阶段的详细计划。**

```
当前阶段实现 ← 同时 → 写下一阶段的详细计划
```

不要一次写完全部计划，因为：
- 后续阶段依赖前面阶段的实际产出和经验
- 提前太多写出的计划大概率需要修改

每个阶段完成后：
1. 确认所有测试通过
2. Docker Compose 一键运行验证
3. 提交并打 tag（v0.1.0, v0.2.0, ...）
4. 更新此文件的状态
5. 再写下一阶段的详细计划

## 会话管理规则

### 新会话启动时

1. 先读本文件了解项目状态
2. 读架构 spec 获取完整技术上下文
3. 读当前阶段的计划文件，定位到要执行的 Task
4. 推荐按 Task 加载计划文件（每个 Task ~200-500 行），避免一次性消耗过多上下文预算

### 会话压缩发生时

- 压缩后对话上下文会被摘要压缩，但文件系统中的所有状态完好
- 恢复步骤：读本文件 → 读 spec → 从 plan 文件中找到当前 Task → 继续
- 已完成的代码修改、git 提交、测试结果都在文件系统中，不依赖对话历史

### 历史会话信息

- 通过 **claude-mem** skill 可以渐进式查找之前会话的信息
- 当实现中需要了解某个决策的背景、某个方案的讨论过程时，使用 claude-mem 检索历史会话
- 不需要预先加载所有历史——按需查找即可

### 并行会话

- 不同阶段可以在不同会话中并行（如 Plan 1 实现 + Plan 2 规划）
- 并行会话之间通过**文件系统**协调，不依赖对话上下文
- 如果需要 worktree 隔离，使用 git worktree

### 上下文预算参考

上下文窗口 200K tokens。关键文档的估算占用：

| 内容 | 估算 tokens | 加载时机 |
|------|------------|----------|
| 本文件 | ~500 | 每次会话 |
| 架构 spec | ~10K | 每次会话 |
| 单个 Task | ~3K-8K | 执行该 Task 时 |
| 全部 Plan 文件 | ~50K | 不推荐，按 Task 加载更高效 |

三个核心文件全加载约 50K tokens（占 25%），技术上可行。但实际实现中会产生大量工具输出（代码、编译、测试），建议按 Task 加载计划文件，把上下文预算留给实现过程。

## 技术约束

- Go Workspace monorepo（`backend/go.work`）
- Connect-RPC（不是 gRPC）用于服务间通信
- Buf 管理 proto 文件
- 有状态服务（User Svc, Community Svc）使用 groupcache 模式
- WS Gateway 是有状态的（conn map + Redis 注册表）
- DM 属于 User Service，群消息属于 Community Service
- 详见架构 spec
