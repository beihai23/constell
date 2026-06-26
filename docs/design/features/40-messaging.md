# 40 · 消息（频道 + DM）

> 这是首个按完整模板写成的 feature 文件，**其结构即为所有 `features/*.md` 的模板**。
> 域前缀：频道 `MSG-*`、DM `DM-*`。实现：`components/chat/{ChannelView,DMChat,MessageList,ChatInput,MessageBubble,ChatHeader}.tsx`、`stores/messagesStore.ts`。

## 0. 概述与用户目标

- **目标**：成员在频道 / DM 中收发消息，本地即时显示，服务端确认后落定；失败可重试；离线禁用；历史可加载与上滑分页。
- **范围**：频道消息与 DM 共用 `MessageList`/`ChatInput`，差异仅在数据源与有无 MemberList。本文件统一描述，差异点显式标注。
- **不在本文件**：成员/presence（`60`）、文件上传细节（`80`）、搜索（`70`）、实时送达/重连（`50`，本文件只引用）。

---

## A. 发送消息

### A1. 屏幕与布局

```
┌─ ChatHeader ────────────────────────┐
│ # general / @peer      [👥][🔍][⋯]   │   ← 频道才显示 👥(toggle成员)
├──────────────────────────────────────┤
│ MessageList（见 B）                    │
├──────────────────────────────────────┤
│ [附件预览行：图/文件卡 + ✕]            │   ← 仅 pendingFiles>0
│ ┌──────────────────────────────────┐ │
│ │[+]  Message …            [Send]   │ │   ← InputGroup：附件键+textarea+发送
│ └──────────────────────────────────┘ │
└──────────────────────────────────────┘
```

### A2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言（测试） |
|---|---|---|---|---|
| 可输入 | 进入有 target 的会话 & WS=CONNECTED | textarea 可用，placeholder「Message #channel」/「Message user」 | — | textarea 可见、可输入 |
| 发送中 | Enter / 点 Send | 气泡立即出现，角标「⏳ Sending…」 | `messageStatus[tempId]=sending` | 气泡可见；无重试按钮 |
| 成功 | ACK 到达 | 角标消失；临时 id/seq 替换为真实值 | `status[id]=sent`；`removeMessageStatus(tempId)` | 气泡无角标 |
| 失败 | 发送抛错/超时 | 红色「✗ Failed — tap to retry」按钮 | `status[tempId]=failed` | 失败按钮可见、可点、**点击后重发** |
| 离线 | `ws !== CONNECTED` | textarea + 附件 + Send 全 disabled；placeholder「Connecting…」 | — | `textarea[disabled]` |
| 无 target | 社区根未选频道 | 输入区 disabled；placeholder「Select a channel to message」 | — | 输入区 disabled |
| 空内容 | 内容为空且无附件 | Send 按钮 disabled | — | Send `disabled` |
| 附件待发 | 选了文件 | 顶部预览行（图缩略/文件卡 + ✕移除） | `pendingFiles[]` | 预览可见；✕可移除 |

### A3. 流程

- **happy**：输入→Enter→乐观插入(tempId, status=sending)→`sendChannelMessage`/`sendDM`→ACK→用真实 id+seq 替换 tempId→status=sent→清空输入。
- **带附件**：选文件→`uploadFile` 拿 fileId→乐观气泡带 attachment→同上。多文件顺序上传。
- **失败**：发送抛错→status=failed→气泡保留→toast「Failed to send message」→点「tap to retry」→重新进入 sending（**当前实现此处断链，见 A6 缺口 1**）。
- **离线**：WS 断→输入区 disabled；恢复→重新可用。
- **大文本**：超 2000 字符→禁发 + 提示（**当前未实现，见 A6 缺口 3**）。

### A4. 验收标准

```
AC MSG-SEND-1  本地即时显示（乐观插入）
  GIVEN 成员在有 channel/peer 的会话 且 WS=CONNECTED
  WHEN 输入 "hi" 并按 Enter
  THEN 该消息气泡立即以 sending 状态显示
  AND 使用临时 id（seq=0）

AC MSG-SEND-2  成功确认
  GIVEN MSG-SEND-1 已执行
  WHEN 服务端 ACK 到达（含 messageId + seq）
  THEN 气泡切换为 sent，移除角标
  AND 真实 messageId/seq 替换临时 id

AC MSG-SEND-3  失败可重试（当前红 — 缺口 1）
  GIVEN MSG-SEND-1 已执行
  WHEN 发送超时或被服务端拒绝
  THEN 气泡显示红色「Failed — tap to retry」
  WHEN 用户点击该提示
  THEN 以原文+原附件重新进入 sending 流程
  AND 成功后落定为 sent

AC MSG-SEND-4  离线禁用
  GIVEN WS != CONNECTED
  THEN textarea、附件键、Send 均 disabled
  AND 占位符为「Connecting…」
  AND Enter 不触发发送

AC MSG-SEND-5  无 target 禁用
  GIVEN 在社区根（未选频道）
  THEN 输入区 disabled，占位「Select a channel to message」

AC MSG-SEND-6  空内容禁发
  GIVEN 内容为空且无附件
  THEN Send 按钮 disabled

AC MSG-SEND-7  实时送达（跨会话，引用 50）
  GIVEN 用户B是该频道成员/DM 对端 且已打开该会话
  WHEN 用户A 发送消息
  THEN 用户B 在 5s 内看到该消息（见 features/50）

AC DM-SEND-1  DM 发送同构
  GIVEN 在 /@me/:peerId 且 WS=CONNECTED
  WHEN 输入并 Enter
  THEN 同 MSG-SEND-1..3 的本地即时/确认/重试行为
```

### A5. 边界与约束

- 单条字符上限：**2000**（超出禁发 + 行内提示；**当前未实现**）。
- 键盘：Enter 发送；Shift+Enter 换行；textarea 自动增高，上限 ~5 行后内部滚动。
- 并发：乐观插入用本地 tempId，ACK 后替换；多消息发送各自独立 tempId。
- 附件：顺序上传；上传失败则整条发送失败（当前把 uploadFile 抛错归入整条失败）。

### A6. 当前实现缺口

1. **【实现·BUG】失败重试断链**：~~`MessageList.tsx:181-185` 渲染 `<MessageBubble>` 时未传 `onRetry`~~ → **已修**：新增 `useMessageSend` hook（封装 reconcile + `retry(tempId)` 重发），MessageList 传 `onRetry={() => retry(msg.id)}`。✅ AC MSG-SEND-3。
2. **【实现】裸色值**：ChatInput/MessageBubble 全程 `#313244/#585b70/#45475a/#cdd6f4/#f38ba8/#181825/#11111b`。**修正**：改 token（见 `00` §6）。
3. **【目标-待建】字符上限**：无 2000 字符校验与提示。→ A5。
4. **【目标-待建】上传进度**：`uploadFile` 无进度反馈，大文件像卡住。→ `features/80`。
5. **【目标-待建】消息编辑/删除**：完全缺失（无右键菜单、无 API 调用）。→ C 段。

---

## B. 消息历史与列表

### B1. 屏幕与布局

`MessageList` 虚拟滚动（`@tanstack/react-virtual`，行高估值 60px，动态测量）。顶部上滑触发分页。

### B2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| 首次加载 | 进入会话 | **骨架**（消息行 skeleton） | `history` pending | ≥3 行骨架可见 |
| 加载完成-有内容 | 历史返回 | 消息列表，滚到底 | `setChannelMessages` | 最新消息可见 |
| 空 | 历史返回且为空 | 居中「No messages yet」 | `messages.length===0` 且非 loading | 空态文案可见 |
| 上滑分页 | scrollTop<200px | 顶部出现「加载更多」指示，prepend 旧消息 | dedup by id | 旧消息出现，滚动位置不跳 |
| 历史失败 | 历史抛错 | 错误条 + 重试 | — | 错误文案 + 重试按钮 |
| 新消息(我在底) | 收到推送 & isNearBottom | 自动滚到底 | — | 新消息可见 |
| 新消息(我不在底) | 收到推送 & !isNearBottom | 不自动滚动；显示「↓ N 条新消息」浮条 | — | 浮条可见，点击回到底 |

### B3. 流程

- **首次加载**：mount→`getChannelHistory`/`getDMHistory`（DESC 分页）→reverse 为 ASC→写入 store→seed 回补游标到 maxSeq。
- **上滑分页**：scrollTop<200px→拉 limit=50→按 id 去重→prepend→保持视口位置。
- **新消息自动滚**：仅当 `isNearBottom`（距底<100px）时 rAF 滚到底。

### B4. 验收标准

```
AC MSG-HIST-1  首次加载显示骨架（当前红 — 缺口 B6-1）
  GIVEN 成员首次进入某频道
  THEN 在历史返回前显示消息行骨架（而非"No messages yet"）

AC MSG-HIST-2  空频道显示空态
  GIVEN 历史返回为空 且非加载中
  THEN 居中显示「No messages yet」

AC MSG-HIST-3  上滑加载更多（当前红 — 缺口 B6-2 无指示）
  GIVEN 已加载一页历史
  WHEN 用户滑到顶部 200px 内
  THEN 显示加载指示并 prepend 更旧的消息
  AND 不产生重复、视口不跳动

AC MSG-HIST-4  历史失败可重试（当前红 — 缺口 B6-3）
  GIVEN 历史请求失败
  THEN 显示错误条 + 重试按钮
  WHEN 点击重试
  THEN 重新请求历史

AC MSG-HIST-5  新消息自动到底
  GIVEN 用户位于列表底部
  WHEN 收到新消息
  THEN 自动滚动使新消息可见

AC MSG-HIST-6  滚动离开底部时不抢焦点（当前红 — 缺口 B6-4）
  GIVEN 用户已上滑离开底部
  WHEN 收到新消息
  THEN 不自动滚动
  AND 显示「↓ N 条新消息」浮条，点击回到底部

AC MSG-HIST-7  历史持久（reload 后仍在）
  GIVEN 用户在频道发过消息
  WHEN 整页 reload 后回到该频道
  THEN 历史消息重新可见（getChannelHistory）
```

### B5. 边界与约束

- 后端返回 DESC，前端 reverse 为 ASC；游标用 seq。
- 虚拟化：行高动态测量（`measureElement`），估值 60px。
- 分页去重以 `id` 为准。

### B6. 当前实现缺口

1. **【实现·BUG】加载态与空态混淆**：~~`MessageList.tsx:145-150` 在历史返回前 `messages.length===0` 直接渲染"No messages yet"~~ → **已修**：引入 `loadState(loading/ready/error)`，加载中显示骨架（`EmptyState`/`ErrorState`/`Skeleton`）。✅ AC MSG-HIST-1。
2. **【实现】上滑分页无加载指示**：~~`handleScroll` 静默拉取~~ → **已修**：`loadingMore` 状态 + 顶部 `pagination-loading` 指示（Spinner + 文案）。✅ AC MSG-HIST-3。
3. **【实现】历史失败静默**：~~`.catch(()=>{})`~~ → **已修**：失败显示 `ErrorState` + 重试（重跑 loadHistory）。✅ AC MSG-HIST-4。
4. **【目标-已建】无「回到底部」浮条** → **已建**：`newCount` + `[data-slot="new-messages-pill"]`（`↓ N new messages`），离开底部收到新消息不抢焦点、显示浮条，点击回到底部；到底部自动清零。✅ AC MSG-HIST-6。**附带修了消息列表的滚动布局 bug**（`min-h-0` 链断裂致列表不滚动、随内容撑高）。
5. **【实现】裸色值**："No messages yet" 用 `#585b70`。改 token。

---

## C. 消息编辑与删除（【目标-待建】整体）

### C2. 状态矩阵（目标态）

| 状态 | 触发 | 期望 UI | 断言 |
|---|---|---|---|
| 右键菜单 | 自己消息右键/悬停 ⋯ | 菜单：Edit / Delete | 菜单项可见 |
| 编辑中 | 点 Edit | 气泡内容变为 textarea + Save/Cancel | 编辑器可见 |
| 已编辑 | Save | 内容更新 + 角标「(edited)」 | 新内容 + edited 标记 |
| 删除确认 | 点 Delete | alert-dialog「确认删除?」 | 确认弹窗 |
| 已删除 | 确认 | 气泡变为「消息已删除」占位（或移除） | 原内容不可见 |

### C4. 验收标准

```
AC MSG-EDIT-1  自己消息可编辑（目标-待建）
  GIVEN 自己发出的消息
  WHEN 右键 → Edit → 改文 → Save
  THEN 内容更新且显示「(edited)」

AC MSG-DEL-1  自己消息可删除（目标-待建）
  GIVEN 自己发出的消息
  WHEN 右键 → Delete → 确认
  THEN 消息移除（或显示已删除占位）

AC MSG-DEL-2  他人消息不可编辑/删除（目标-待建）
  GIVEN 别人发的消息
  THEN 右键菜单不含 Edit/Delete
```

### C6. 当前实现缺口

- **删除（MSG-DEL-1/2）已建**：proto 新增 `DeleteMessage` RPC（author-only，service 校验 author），api-gateway `DELETE /channels/{cid}/messages/{mid}`，SDK `deleteChannelMessage`，MessageBubble 自己消息 hover 显示删除键 → MessageList 确认 Dialog → DB 删除（reload 后仍消失）。非作者无删除入口（UI + 后端 403 双重）。✅ AC MSG-DEL-1 / MSG-DEL-2。
- **编辑（MSG-EDIT-1）已建**：proto `EditMessage` RPC（author-only），api-gateway `PATCH /channels/{cid}/messages/{mid}`，SDK `editChannelMessage`，repo `UpdateChannelMessage`（`updated_at = GREATEST(NOW(), created_at + 1s)` 保证 `(edited)` 标记可靠），MessageBubble 自己消息 hover → ✏️ → 内联编辑器 → 保存；`(edited)` 标记由 `updated_at > created_at` 派生，附件保留。✅ AC MSG-EDIT-1。
- **实时删除传播（其他在线客户端）**：当前删除只对操作者立即生效；其他客户端下次 history 加载/刷新才消失（DB 已删）。实时推送删除事件是后续 AC（参考 message.created 链路）。

---

## D. DM 特有

### D2. 状态矩阵（仅列与频道差异点）

| 状态 | 期望 UI | 断言 |
|---|---|---|
| DM 标题 | ChatHeader 显示对端昵称 + 在线点 | 昵称 + 在线点可见 |
| 无 MemberList | DM 不渲染右侧成员列表 | 无 MemberList |
| mark-read | 进入 DM 清未读 | 未读角标清零 |

### D6. 当前实现缺口

1. **【实现】服务端 mark-read 未接**：`DMChat.tsx:20-22` 注释承认「unread store 用 peerId，markDMRead 期望 conversationId，暂只清本地未读」。**修正**：解析 conversationId 并调服务端 mark-read（与 `features/50` 未读一致）。
2. **【实现】DMChat 无加载/错误态**：与 ChannelView 不同，DMChat 无 `getChannels` 等价加载，但 MessageList 共用，故骨架缺口同 B6-1。

---

## 7. 待定问题

- 编辑/删除是否要服务端 RPC？（影响 C 段能否落地，待查 `proto/`）
- 消息撤回（区别于删除）是否需要？暂列 future。
- 「输入中」指示见 `features/50`（realtime）还是独立？归 `features/50`。
