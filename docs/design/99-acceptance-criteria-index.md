# 99 · 验收标准索引（L5，测试 ↔ AC 1:1）

> 全局平铺所有 AC ID，供测试逐条对照。每条测试**必须**以其覆盖的 AC ID 命名（如 `test('MSG-SEND-3 失败可重试', …)`）。
> 状态图例：✅ 已实现（可立即写测试） · 🔴 BUG（当前行为错，测试先红→修代码） · 🟡 目标-待建（测试先红→驱动实现） · ⚠️ 待核（依赖后端/SDK，需先确认）

## 状态总览

- 总计 **75** 条 AC。
- ✅ 已实现 + 已锁测试：**74**（9 BUG + 9 目标-已建全落地 + FILE-SEND-1/2/VIEW 全链路验证）
- 🔴 BUG（实现错误）：**0**
- 🟡 目标-待建：**0**
- ⚠️ 待核（SDK 内部）：**1**（RT-CONN-1：SDK 是否在重连时正确置 Connecting，需核 ws-manager）

> 第 1 轮（2026-06-25）：TDD 闭环全部 9 个历史 BUG（见下「已修复清单」）+ 新增原语 `Spinner`/`EmptyState`/`ErrorState`、`useMessageSend` hook。
> 第 2 轮（2026-06-25）：推进高价值 🟡 —— 复活 `SearchDialog` 为 ⌘K 全局面板（SEARCH-GLOBAL-1/3）、新消息「回到底部」浮条（MSG-HIST-6，并顺手修了消息列表不滚动的布局 bug）、上传 25MB 大小校验（FILE-SIZE-1）。每条 TDD（先红后绿）。
> 第 3 轮（2026-06-25，用户报告 + 连带发现）：
> - **未读累加 bug**（用户报告）：正在查看的频道收到消息仍累加未读、rail 误显角标。根因 `useClientEvents` 无脑 increment + 无激活频道追踪 + `currentChannelId` 死字段。补 AC **UNREAD-1b**，修：`uiStore.activeChannelId/activePeerId` + useClientEvents 跳过激活会话。
> - **频道选中高亮一直坏**（死字段 `currentChannelId` 从未被 set）：CHAN-LIST-2 的选中态此前恒 false。修：ChannelList 改从路由派生 + `aria-current`；删 `communitiesStore` 死字段 `currentChannelId/currentCommunityId/selectChannel/selectCommunity`。
> - **search-and-join 测试结构性坏掉**（locator 错，非 flake）：社区结果行是 `<div>` 内含 `<button>Join</button>`，原 `getByRole('button',{name:/社区名/})` 永不匹配 → 测试从不执行。修 locator + 对异步索引加轮询。**全套首次 40/40 全绿。**
> 第 4 轮（2026-06-25）：**离开社区 COMM-LEAVE-1** 全栈打通——api-gateway 新增 `DELETE /communities/{id}/leave`（→ LeaveCommunity RPC，owner 被拒）+ SDK `leaveCommunity` + ChannelList 头部 Leave 按钮 + 确认 Dialog；`--no-cache` 重建 api-gateway 容器（普通 `--build` 命中 BuildKit 缓存没生效，是个坑）。**全套 41/41 绿。**

## 10 · 认证（AUTH）

| ID | 一句话 | 状态 |
|---|---|---|
| AUTH-REG-1 | 注册成功进入应用 | ✅ |
| AUTH-REG-2 | 密码不一致被拦 | ✅ |
| AUTH-REG-3 | 注册失败有反馈 | ✅ |
| AUTH-LOGIN-1 | 登录成功进入应用 | ✅ |
| AUTH-LOGIN-2 | 凭证错误有反馈 | ✅ |
| AUTH-SESS-1 | 会话恢复（刷新不掉登录） | ✅ |
| AUTH-SESS-2 | 失效自动登出 | ✅ |
| AUTH-LOGOUT-1 | 登出回到登录页 | ✅ |

## 20 · 社区（COMM）

| ID | 一句话 | 状态 |
|---|---|---|
| COMM-CREATE-1 | 创建社区并进入默认频道 | ✅ |
| COMM-CREATE-2 | 创建失败有反馈 | ✅ |
| COMM-RAIL-1 | 未读聚合角标 | ✅ |
| COMM-RAIL-2 | 选中态可视 | ✅ |
| COMM-JOIN-1 | 从搜索加入公开社区 | ✅ |
| COMM-JOIN-2 | 行点击不误加入 | ✅ |
| COMM-LEAVE-1 | 离开社区 | ✅（建：api-gateway /leave + SDK + ChannelList UI + 确认） |

## 30 · 频道（CHAN）

| ID | 一句话 | 状态 |
|---|---|---|
| CHAN-LIST-1 | 频道列表显示 | ✅ |
| CHAN-LIST-2 | 选中态与未读角标 | ✅ |
| CHAN-LIST-3 | 本地过滤 | ✅ |
| CHAN-LIST-4 | 无频道空态 | ✅ |
| CHAN-CREATE-1 | 创建频道并进入 | ✅ |
| CHAN-CREATE-2 | 名称长度约束 | ✅ |
| CHAN-CREATE-3 | 创建失败有反馈 | ✅ |
| CHAN-VISIT-1 | 非成员访问私有频道被拒 | ✅ |

## 40 · 消息（MSG / DM）

| ID | 一句话 | 状态 |
|---|---|---|
| MSG-SEND-1 | 本地即时显示（乐观插入） | ✅ |
| MSG-SEND-2 | 成功确认 | ✅ |
| MSG-SEND-3 | 失败可重试 | ✅（修：useMessageSend.retry + onRetry 接线） |
| MSG-SEND-4 | 离线禁用 | ✅ |
| MSG-SEND-5 | 无 target 禁用 | ✅ |
| MSG-SEND-6 | 空内容禁发 | ✅ |
| MSG-SEND-7 | 实时送达（跨会话） | ✅ |
| DM-SEND-1 | DM 发送同构 | ✅ |
| MSG-HIST-1 | 首次加载显示骨架 | ✅（修：loadState + Skeleton） |
| MSG-HIST-2 | 空频道显示空态 | ✅ |
| MSG-HIST-3 | 上滑加载更多 | ✅（修：loadingMore + pagination-loading 指示） |
| MSG-HIST-4 | 历史失败可重试 | ✅（修：ErrorState + 重试） |
| MSG-HIST-5 | 新消息自动到底 | ✅ |
| MSG-HIST-6 | 滚动离开底部不抢焦点 + 回到底部浮条 | ✅（建：newCount + new-messages-pill） |
| MSG-HIST-7 | 历史持久（reload 后仍在） | ✅ |
| MSG-EDIT-1 | 自己消息可编辑 | ✅（建：proto EditMessage + api-gateway + SDK + 内联编辑器 + (edited) 标记） |
| MSG-DEL-1 | 自己消息可删除 | ✅（建：proto DeleteMessage + api-gateway + SDK + UI 确认） |
| MSG-DEL-2 | 他人消息不可编辑/删除 | ✅（建：仅自己消息显示删除键 + 后端 author 校验 403） |

## 50 · 实时（RT / PRES / UNREAD / SYNC）

| ID | 一句话 | 状态 |
|---|---|---|
| RT-CONN-1 | 连接中可见 | ⚠️ 依赖 SDK 置 Connecting |
| RT-CONN-2 | 断线可见且自动重连 | ✅ |
| RT-CONN-3 | 重连后回补丢失消息 | ✅ |
| PRES-1 | 在线状态实时更新 | ✅ |
| PRES-2 | presence 按需拉取 | ✅ |
| UNREAD-1 | 非当前会话累计未读 | ✅ |
| UNREAD-2 | 打开会话清未读 | ✅ |
| SYNC-1 | tab 可见触发回补 | ✅ |
| SYNC-2 | 跨网关实时送达 | ✅ |

## 60 · 成员（MEM）

| ID | 一句话 | 状态 |
|---|---|---|
| MEM-LIST-1 | 成员列表分组与计数 | ✅ |
| MEM-LIST-2 | 加载显示骨架 | ✅（修：loadState + Skeleton） |
| MEM-LIST-3 | 成员加载失败可重试 | ✅（修：ErrorState + 重试） |
| MEM-PRES-1 | 在线状态实时迁移 | ✅ |
| MEM-TOGGLE-1 | 成员列表默认隐藏 + 可切换 | ✅ |
| MEM-PROFILE-1 | 头像资料卡 | ✅（建：MemberProfileDialog + getUser + Send DM） |
| MEM-KICK-1 | owner 踢出成员 | ✅（修：后端 KickMember 已就绪；前端 UI 仍 🟡 目标） |
| MEM-KICK-2 | 非 owner 不能踢人 | ✅（建：MemberProfileDialog Kick 仅 owner 可见） |

## 70 · 搜索（SEARCH）

| ID | 一句话 | 状态 |
|---|---|---|
| SEARCH-FILTER-1 | 内联过滤频道 | ✅ |
| SEARCH-GLOBAL-1 | ⌘K 打开全局面板 | ✅（建：SearchDialog 挂载 + ⌘K 切换） |
| SEARCH-GLOBAL-2 | 跨类结果 | ✅（Enter 内联） |
| SEARCH-GLOBAL-3 | 空查询引导 | ✅（随面板复活） |
| SEARCH-GLOBAL-4 | 无结果 | ✅ |
| SEARCH-GLOBAL-5 | 搜索失败可见 | ✅（修：searchError + ErrorState 重试） |
| SEARCH-GLOBAL-6 | 结果跳转 | ✅ |
| SEARCH-GLOBAL-7 | Esc 关闭 | ✅ |
| SEARCH-JOIN-1 | 从结果加入 | ✅ |

## 80 · 文件（FILE）

| ID | 一句话 | 状态 |
|---|---|---|
| FILE-PICK-1 | 选择文件生成预览 | ✅ |
| FILE-REMOVE-1 | 移除待发文件 | ✅ |
| FILE-SEND-1 | 带图发送 | ✅（curl 实测：upload→file_id→带 file_ids 发消息 OK，"作弊 6" 早已修） |
| FILE-SEND-2 | 纯附件发送 | ✅（同上） |
| FILE-UPLOAD-1 | 上传进度可见 | ✅（建：SDK XHR uploadWithProgress + 预览进度条） |
| FILE-UPLOAD-2 | 上传失败可重试 | ✅（修：per-file 失败分离，保留预览 + upload-error） |
| FILE-SIZE-1 | 超大拒绝 | ✅（建：25MB 校验 + toast） |
| FILE-VIEW-1 | 接收方查看 | ✅（修：MINIO_BASE_URL 可达 + bucket public + GetMessages 带 attachments + JOIN file_metadata 补元数据 + api-gateway 重建 url） |

---

## ✅ 已修复清单（原 9 个 BUG，过去"作弊"掩盖的真实问题，本轮 TDD 全部闭环）

| AC | 问题 | 修法 | 测试 |
|---|---|---|---|
| MSG-SEND-3 | 失败重试断链（MessageList 未传 onRetry） | `useMessageSend` hook + onRetry 接线 | msg-retry.spec |
| MSG-HIST-1 | 加载态/空态混淆（显示"No messages yet"） | loadState + Skeleton | msg-history.spec |
| MSG-HIST-3 | 上滑分页无加载指示 | loadingMore + pagination-loading | msg-paginate.spec |
| MSG-HIST-4 | 历史失败静默 | ErrorState + 重试 | msg-history.spec |
| MEM-LIST-2 | 成员列表加载态/空态混淆 | loadState + Skeleton | member-list.spec |
| MEM-LIST-3 | 成员加载失败静默 | ErrorState + 重试 | member-list.spec |
| MEM-KICK-1 | owner 踢人后端 BUG | 后端已用 KickMember（前端 UI 仍 🟡） | member-kick.spec |
| SEARCH-GLOBAL-5 | 搜索失败静默 | searchError + ErrorState 重试 | search-fail.spec |
| FILE-UPLOAD-2 | 上传失败被吞进整条发送失败 | per-file 失败分离 + 保留预览 | file-upload-fail.spec |

> 另：本轮顺手修了一个**预先存在的 `npm run build` 破损**（`ChannelList` search-join 改动漏接 store selector + 变量遮蔽），现 build 干净。

### 仍待处理（非本轮 scope）

- 全量"裸十六进制"违反设计系统（`00` §6）——一致性回归，建议后续集中清理。
- 🟡 剩余 **0** 条目标-待建。⚠️ 1 条待核：RT-CONN-1（SDK 重连时 Connecting 置位，需核 ws-manager）。

## 测试纪律（再次强调）

- 测试用 AC ID 命名；一个测试覆盖 ≥1 个 AC ID。
- 行为与 spec 冲突 → **显式二选一**：修代码（更新本表状态 ✅），或改 spec（记录理由、更新 AC）。**禁止**为变绿而：放宽断言、吞 `.catch(()=>{})`、`skip`、改测试迁就实现。
- 新增交互 → 先在该 feature 文件加 AC（本表登记），再写测试。

## 测试方法约定（怎么可靠地触发各状态）

环境：后端栈跑在 Docker（api:8080 / pg:15432 / redis:16379 / nats / 双 ws-gateway）；前端用 `npm run dev`（vite，HMR）代理 `/api`→:8080、`/ws`→:8081。跑测试：`E2E_BASE_URL=http://localhost:5174 npx playwright test`。Playwright 1.60。

- **REST 失败/延迟**（MSG-HIST-1/3/4、MEM-LIST-2/3、SEARCH-GLOBAL-5、FILE-UPLOAD-2）：`page.route('**/api/v1/...', route => route.fulfill({ status: 500 }))` 注入失败；`route.fulfill` 前加 `await new Promise(r => setTimeout(r, 2000))` 注入延迟以观察加载态/骨架。
- **WS 行为**（MSG-SEND-1/2/3/7、RT-CONN-*、PRES/UNREAD/SYNC）：`page.routeWebSocket(/.*/, ws => ...)` 拦截。在 handler 里可：`ws.connectToServer()` 透传；或自行 `ws.onServerMessage`/`ws.send` 模拟 ACK/推送/拒绝/超时——MSG-SEND-3 的「失败→点重试→重发」由控制是否回 ACK 实现。
- **AC 状态约定**：🔴 bug 的测试先写成「期望目标态」（红）→ 修代码 → 转 ✅。每修一个，更新本索引对应行 + 状态总览计数。
