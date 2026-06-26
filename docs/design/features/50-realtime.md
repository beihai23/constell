# 50 · 实时（连接 / 重连 / presence / 未读 / 送达）

> 域前缀 `RT-*`（连接/重连）、`PRES-*`（presence）、`UNREAD-*`（未读）、`SYNC-*`（回补/送达）。
> 实现：`hooks/useClientEvents.ts`、`hooks/useMessageSync.ts`、`hooks/useInitialData.ts`、`hooks/usePullPresence.ts`、`components/layout/ConnectionStatusBar.tsx`、`stores/uiStore.ts`、`stores/unreadStore.ts`、`stores/syncStore.ts`。底层 SDK `WSStatus`。

## 0. 概述与用户目标

- 连接状态始终可感知（不藏「断线」）；断线自动重连并回补丢失消息；presence 与未读数准确；跨 ws-gateway 实例仍能送达。用户目标：消息不丢、状态不骗人。

## 1. 屏幕与布局

- 顶部 `ConnectionStatusBar`（仅 WS≠CONNECTED 可见）。
- 输入区随连接态 disable（见 `40` A2）。
- presence 点：MemberList、DMList、ChatHeader(DM)。
- 未读角标：CommunityRail（社区聚合）、ChannelList（频道/DM）、DMList。

## 2. 状态矩阵

### 2a. 连接状态（`uiStore.wsStatus`）

| wsStatus | 触发 | 期望 UI | 断言 |
|---|---|---|---|
| `Connected` | SDK `connected` 事件 | 状态条隐藏；输入区可用 | 无状态条；textarea 可用 |
| `Connecting` | 连接建立中 | 黄色顶条「Connecting…」；输入 disable | 顶条可见；textarea disabled |
| `Reconnecting` | 断线后自动重连中 | 红色顶条「Reconnecting…」；输入 disable | 顶条可见 |
| `Disconnected` | SDK `disconnected` 事件 | 红色顶条「Disconnected — attempting to reconnect…」；输入 disable | 顶条可见 |

### 2b. presence / 未读 / 回补

| 状态 | 触发 | 期望 UI | 断言 |
|---|---|---|---|
| 在线 | `user_online` 推送 或 拉取命中 | 成员/DM 头像绿点；MemberList ONLINE 组 | 绿点可见 |
| 离线 | `user_offline` 或 拉取未命中 | 灰点；移入 OFFLINE 组 | 灰点 |
| 有未读 | 收到消息且非当前会话 | 对应频道/DM/社区角标 +1 | 角标计数 |
| 清未读 | 打开会话 | 该会话角标清零 | 角标消失 |
| 回补中 | (re)connect / 可见 / 30s | （目标）非阻塞微指示；消息静默合并 | 新漏消息最终出现 |

## 3. 流程

- **连接生命周期**：登录→`client.connect()`→WS 建链→`connected`→`wsStatus=Connected`；断开→`disconnected`→`Disconnected`→SDK 自动重连→`Reconnecting`/`Connecting`→`Connected`。
- **实时推送**：`useClientEvents` 订阅 `channel_message`/`dm_received`/`user_online`/`user_offline`→写 store→UI 增量更新；非当前会话的消息 `incrementUnread`。
- **回补（防推送丢失）**：`useMessageSync` 在 (re)connect、tab 可见、每 30s，对每个已加载 scope 用 `sinceSeq` 拉取并 `merge`，游标单调前进。
- **presence 双通道**：push（`user_online/offline`）更新已渲染视图；pull（`getPresence`）按需为新视图（搜索结果、新 DM）取真值。
- **跨网关送达**：多 ws-gateway 实例时，消息经 NATS 扇出到目标用户所在实例，再下推。前端无感知，由后端保证。

## 4. 验收标准

```
AC RT-CONN-1  连接中可见
  GIVEN WS 正在建链
  THEN 顶部黄色「Connecting…」条可见
  AND 输入区 disabled

AC RT-CONN-2  断线可见且自动重连
  GIVEN 已连接后网络断开
  THEN 顶部红色条可见（Disconnected/Reconnecting）
  WHEN 网络恢复
  THEN 自动回到 Connected（顶条消失）

AC RT-CONN-3  重连后回补丢失消息
  GIVEN 用户A 断线期间，频道有新消息 M
  WHEN A 重连
  THEN A 在重连后看到 M（经 sinceSeq 回补）

AC PRES-1  在线状态实时更新
  GIVEN 用户A、B 互为同社区成员，均在线
  WHEN B 上线
  THEN A 的 MemberList ONLINE 组出现 B

AC PRES-2  presence 按需拉取
  GIVEN A 打开一个新视图（如搜索结果含用户C）
  WHEN C 的头像渲染
  THEN 拉取 C 的 presence 并正确显示在线/离线

AC UNREAD-1  非当前会话累计未读
  GIVEN 用户A 在 #general，#random 收到 2 条新消息
  THEN #random 角标=2，且其社区聚合角标=2

AC UNREAD-1b 正在查看的会话不累计未读（2026-06-25 补：原 AC 漏了这个隐含语义）
  GIVEN 用户A 正在查看 #general（页面已打开该频道）
  WHEN 别人往 #general 发消息
  THEN #general 不产生未读，社区聚合角标也不含它
  （未读 =「没看到的消息」；正在看 = 已看到。实现：uiStore.activeChannelId/activePeerId，useClientEvents 跳过激活会话的 increment。）

AC UNREAD-2  打开会话清未读
  GIVEN #random 有未读
  WHEN A 打开 #random
  THEN #random 与社区聚合角标均减少/清零

AC SYNC-1  tab 可见触发回补
  GIVEN A 切到别的 tab 期间收到消息
  WHEN A 切回（visibility=visible）
  THEN 触发回补，缺失消息出现

AC SYNC-2  跨会话实时送达（引用 MSG-SEND-7）
  GIVEN B 是成员/DM 对端 且已打开会话
  WHEN A 发送消息
  THEN B 在 5s 内看到（无论 A、B 是否在同一 ws-gateway 实例）
```

## 5. 边界与约束

- 回补 `limit=200`、轮询 `30s`（`useMessageSync.ts:6-7`）。
- 游标 `seq` 单调；`advanceDM/Channel` 只前进不后退。
- presence 以 Redis 为真值；push 是优化。
- `Connecting`/`Reconnecting` 由 SDK connect 流程置位（非事件桥接），需确认 SDK 在重连时确实发出对应状态（见缺口 1）。

## 6. 当前实现缺口

1. **【实现·待核】Connecting/Reconnecting 置位路径**：`useClientEvents` 只处理 `connected`→Connected、`disconnected`→Disconnected 两个事件；`Connecting`/`Reconnecting` 依赖 SDK 内部 `setWsStatus`。**修正/核实**：确认 SDK 在 `connect()` 与重连循环中正确置 Connecting/Reconnecting，否则 UI 会卡在 Disconnected 文案。→ AC RT-CONN-1/2。
2. **【实现】回补无可见指示**：`useMessageSync` 失败仅 `console.warn`，用户看不到「正在同步」。**目标态**：非阻塞微指示（可选）。→ §2b。
3. **【实现】裸色值**：`ConnectionStatusBar.tsx:22-23` 用 `#f9e2af/#f38ba8/#1e1e2e`。**修正**：黄=`text-background bg-[Yellow]`、红=`bg-destructive`（见 `00` §2 状态色约定）。
4. **【实现】presence/未读拉取静默失败**：`useInitialData`/`usePullPresence` 多处 `catch{}`（非关键，可接受），但首屏失败时无重试。**目标态**：首屏关键数据（社区）已有 toast，其余可接受乐观。
5. **【目标-待建】无手动「重连」按钮**：长时间 Disconnected 时用户无主动操作入口。→ 可在状态条加「重试连接」。

## 7. 待定问题

- 「输入中（typing）」指示：归本域还是独立？**判断**：归本域，作为【目标-待建】AC（typing 推送通道），后端需支持。
- 离线消息队列（断线期间发出的消息暂存重发）：当前由乐观+failed+重试覆盖（见 `40` A），是否需更结构化的 outbox？暂不需要。
