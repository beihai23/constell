# 60 · 成员（列表 / presence / 踢出 / 资料）

> 域前缀 `MEM-*`。实现：`components/layout/MemberList.tsx`、`components/chat/ChatHeader.tsx`(成员列表 toggle)、`hooks/usePullPresence.ts`。踢出走 REST（见缺口 1）。

## 0. 概述与用户目标

查看社区成员、看到谁在线、点头像看资料、owner 能踢人。用户目标：知道「这个社区里有谁、谁在」；管理者能维护成员。

## 1. 屏幕与布局

- **右列 MemberList（240px，仅频道视图 + `showMemberList=true`）**：MEMBERS — N → ONLINE 分组 → OFFLINE 分组。
- **ChatHeader members toggle**：`👥` 按钮，切换 `showMemberList`。
- **MemberRow**：头像 + 在线点 + 昵称（【目标】悬停/点击出资料卡）。

## 2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| 加载中 | getMembers 进行中 | （目标）骨架行 | loading | 骨架 |
| 有成员 | 成员返回 | ONLINE/OFFLINE 分组，各带计数 | `members[]` | 分组+计数可见 |
| 全空 | 成员为空 | 「No members found」 | `members.length=0` | 文案 |
| 在线 | presence 命中 | 绿点 + ONLINE 组 | `onlineUsers.has(id)` | 绿点 |
| 离线 | presence 未命中 | 灰点 + OFFLINE 组 | — | 灰点 |
| 资料卡 | 悬停/点头像（目标） | popover：昵称+邮箱+在线+（DM 入口） | — | 卡片可见 |
| 踢出-确认 | owner 对成员触发 Kick（目标） | alert-dialog「确认踢出 X?」 | — | 确认弹窗 |
| 踢出-成功 | 确认 | toast；该成员从列表消失 | — | 成员消失 |
| 默认隐藏 | 进入频道 | MemberList 不渲染（需手动 toggle） | `showMemberList=false` | 无右列 |

## 3. 流程

- **加载**：`ChannelView` 进入社区 → `MemberList` mount → `getMembers` → `usePullPresence` 拉真值 → 分组渲染。
- **实时**：`user_online/offline` 推送 → `onlineUsers` 变 → 重新分组。
- **踢出**（目标）：owner 在成员上触发 Kick → alert-dialog 确认 → `DELETE /communities/{id}/members/{uid}` → 成员移除。

## 4. 验收标准

```
AC MEM-LIST-1  成员列表分组与计数
  GIVEN 社区有 2 在线、3 离线成员
  THEN MemberList 显示「MEMBERS — 5」「ONLINE — 2」「OFFLINE — 3」

AC MEM-LIST-2  加载显示骨架（当前红 — 缺口 2）
  GIVEN 首次进入社区
  THEN 成员返回前显示骨架行（而非空态）

AC MEM-LIST-3  成员加载失败可重试（当前红 — 缺口 3）
  GIVEN getMembers 失败
  THEN 显示错误 + 重试，而非静默空

AC MEM-PRES-1  在线状态实时迁移
  GIVEN 离线成员 B 上线
  WHEN 收到 user_online
  THEN B 从 OFFLINE 移到 ONLINE 组

AC MEM-TOGGLE-1  成员列表默认隐藏 + 可切换
  GIVEN 用户进入频道
  THEN 默认无 MemberList
  WHEN 点 ChatHeader 👥
  THEN MemberList 出现；再点消失

AC MEM-PROFILE-1  头像资料卡（目标-待建）
  GIVEN 成员列表/消息头像
  WHEN 悬停/点击某成员头像
  THEN popover 显示其昵称、邮箱、在线状态

AC MEM-KICK-1  owner 踢出成员（当前红 — 缺口 1 后端 BUG）
  GIVEN owner 与成员 victim 同社区
  WHEN owner 对 victim 发起踢出并确认
  THEN victim 从成员列表消失
  AND victim 再访问该社区被拒

AC MEM-KICK-2  非 owner 不能踢人（目标）
  GIVEN 非 owner 成员
  THEN 对他人无 Kick 入口（或操作被服务端拒）
```

## 5. 边界与约束

- presence 以 Redis 为真值；push 为优化。
- owner 不能被踢/不能离开（约束需明确）。
- 成员权限由 `@everyone` 角色（position=0）决定（见记忆 [[e2e-test-report-2026-06-09]] 作弊 1 的角色分配 bug）。

## 6. 当前实现缺口

1. **【实现·BUG（后端）】owner 踢人不可用**：~~`api-gateway` 的 `RemoveMember` handler 调 `LeaveCommunity`（忽略 URL `{uid}`）~~ → **已修**：handler 现调 `KickMember(communityId, uid)`（`community_handler.go:444`），community-service 带权限校验（owner 或 KickMembers 权限）。✅ AC MEM-KICK-1（后端契约已锁测试；前端 kick UI 仍为【目标-待建】）。
2. **【实现】MemberList 无加载骨架**：~~`MemberList.tsx:25-33` 返回前 `members=[]` → 显示「No members found」~~ → **已修**：`loadState(loading/ready/error)` + 骨架 + `ErrorState` 重试。✅ AC MEM-LIST-2/3。
3. **【实现】getMembers 静默失败**：~~`.catch(()=>{})`~~ → **已修**（同上，失败显示 `ErrorState` + 重试）。✅ AC MEM-LIST-3。
4. **【实现】MemberList 裸色值**：`#181825/#313244/#585b70/#cdd6f4/#1e1e2e/#a6e3a1`。改 token。
5. **【目标-已建】无成员资料卡** → **已建**：MemberRow 点击 → `MemberProfileDialog`（getUser 拉 nickname/email，显示在线态 + Send DM）。用 Dialog 实现（@base-ui 无 popover 原语，复用 Dialog 最稳）。✅ AC MEM-PROFILE-1。
6. **【实现·可发现性】成员列表默认隐藏**：新用户不知道有成员列表。保留默认隐藏（节省空间），但 toggle 按钮加 Tooltip「Members」（待补）。→ AC MEM-TOGGLE-1。
7. **【目标-已建】owner 踢人 UI（MEM-KICK-1 UI）/ 非 owner 无踢人入口（MEM-KICK-2）** → **已建**：MemberProfileDialog 在 `canKick`（当前用户是 community owner）时显示 Kick 按钮（两步确认），调 SDK `kickMember`；非 owner 看不到。SDK 新增 `kickMember`。✅ AC MEM-KICK-2（+ MEM-KICK-1 的 UI 半边）。
8. **【目标-待建】角色/权限 UI**（@everyone、自定义角色）—— 列 future。

## 7. 待定问题

- KickMember 后端 RPC 是否已实现？（决定 AC MEM-KICK-1 红多久）
- 资料卡是否含「发 DM / 加好友」入口？（建议含 DM 入口）
