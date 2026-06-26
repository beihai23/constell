# 02 · 组件库：解剖与状态空间（L2）

> 原语的解剖、变体、**完整状态空间**的唯一真相源。状态空间记号：✅ 已有视觉样式、⚠️ 有逻辑无视觉、❌ 缺失。
> 原语位于 `clients/web/src/components/ui/*`（底层 `@base-ui/react`）。复合组件在本文件末尾给索引，深度状态矩阵在对应 `features/*.md`。

## 原语状态词汇（统一约定）

每个交互原语都应覆盖以下状态；下文逐组件标注实际覆盖情况：

`rest` · `hover` · `focus-visible` · `active` · `disabled` · `aria-invalid`(error) · `loading` · `selected/aria-expanded`

---

## 1. Button — `ui/button.tsx`

- 变体（`buttonVariants`，`button.tsx:10-21`）：`default`(primary) · `outline` · `secondary` · `ghost` · `destructive` · `link`
- 尺寸：`default`(h-8) · `xs`(h-6) · `sm`(h-7) · `lg`(h-9) · `icon`(8×8) · `icon-xs` · `icon-sm` · `icon-lg`
- 状态空间：
  - ✅ hover（各变体有 `hover:`）、✅ focus-visible（`ring-ring/50`）、✅ active（`translate-y-px`）、✅ disabled（`opacity-50 pointer-events-none`）、✅ aria-invalid（destructive 环）
- **⚠️ loading 态无内置样式**：当前靠调用方改文案（"Creating…"）+ `disabled`。**目标态**：增加 `loading` prop（内置 spinner，自动 disabled，保留文案），全表单统一。属【目标-待建】原语增强。

## 2. Input — `ui/input.tsx`

- 单行输入，h-8，圆角 `rounded-lg`，`bg-input/30`(dark)。
- 状态：✅ hover/focus ✅ disabled(`bg-input/50`) ✅ aria-invalid(destructive)。✅ placeholder=`text-muted-foreground`。

## 3. Textarea — `ui/textarea.tsx`

- `field-sizing-content` 自动增高，`min-h-16`。
- 状态：✅ hover/focus ✅ disabled ✅ aria-invalid。✅ placeholder。
- **用途约定**：消息输入用 `InputGroupTextarea`（见 §8）而非裸 Textarea。

## 4. Dialog — `ui/dialog.tsx`

- 组成：`Dialog`(root) · `DialogTrigger` · `DialogContent`(含 `DialogOverlay` 遮罩 + 可选关闭键) · `DialogHeader` · `DialogTitle` · `DialogDescription` · `DialogFooter`。
- 视觉：居中 `bg-popover`，`max-w-sm`，遮罩 `bg-black/10 + backdrop-blur-xs`，开合 `fade+zoom 95%`。
- 状态：✅ open/closed 动效。✅ Esc/外点关闭（base-ui 内置）。✅ showCloseButton。
- **⚠️ 无 `alert-dialog` 变体**：危险确认（删除/踢出/离开）目前复用 Dialog，但语义上应「模态阻断 + 默认聚焦取消」。**目标态**：新增 `alert-dialog` 原语（`@base-ui/dialog` 的 `modal`），见 `features/40`、`60`。

## 5. Avatar — `ui/avatar.tsx`

- 组成：`Avatar`(root, size: `sm`/`default`/`lg`) · `AvatarImage` · `AvatarFallback` · `AvatarBadge`(右下角标，如在线点) · `AvatarGroup` · `AvatarGroupCount`。
- 状态：✅ image/fallback 切换。✅ AvatarBadge（可放在线点图标）。
- **用途约定**：presence 在线点用 `AvatarBadge` + Green；勿另造绝对定位 div。

## 6. Badge — `ui/badge.tsx`

- 变体：`default` · `secondary` · `destructive` · `outline` · `ghost` · `link`。h-5，`rounded-4xl`。
- **⚠️ 未读角标未用 Badge**：`CommunityRail.tsx:234` 用裸 `<span>` + `#f38ba8`。**目标态**：未读角标用 `Badge variant="destructive"`（或保留胶囊形态但用 token 色）。

## 7. Skeleton — `ui/skeleton.tsx`

- `animate-pulse rounded-md bg-muted`。**唯一的加载视觉原语**。
- **⚠️ 无行内 spinner**：需一个非阻塞的小 spinner 原语（按钮 loading、行内「加载更多」）。**目标态**：新增 `spinner`（见 `00` §8）。

## 8. InputGroup — `ui/input-group.tsx`

- 组成：`InputGroup`(容器) · `InputGroupAddon`(align: `inline-start`/`inline-end`/`block-start`/`block-end`) · `InputGroupButton` · `InputGroupText` · `InputGroupInput` · `InputGroupTextarea`。
- 状态：✅ 容器随内部 control 的 focus/aria-invalid/disabled 联动变色。
- **用途约定**：消息输入区（附件键 + textarea + 发送键）用此组件。见 `features/40-messaging.md` 的线框。

## 9. ScrollArea — `ui/scroll-area.tsx`

- 自定义滚动条（`bg-border` thumb，w-2.5/h-2.5）。消息列表、成员列表、频道列表均应用此组件包裹以统一滚动外观。

## 10. Tooltip — `ui/tooltip.tsx`

- `TooltipProvider`(delay=0) · `Tooltip` · `TooltipTrigger` · `TooltipContent`(side/sideOffset/align)。`bg-foreground text-background`。
- **⚠️ 覆盖不全**：图标按钮（发送、成员列表 toggle、创建频道「+」）多靠 `aria-label`/`title` 而非 Tooltip。**目标态**：所有纯图标按钮加 Tooltip（可发现性）。

## 11. Separator — `ui/separator.tsx`

- `bg-border`，horizontal/vertical。Rail 用作分组分隔（但用了裸色 `bg-[#313244]`，应改 `bg-border`）。

## 12. Command (cmdk) — `ui/command.tsx`

- 命令面板原语（`cmdk`）。**当前仅 `SearchDialog.tsx`（死代码）使用**。
- **目标态**：⌘K 全局搜索面板复活后作为其容器（见 `features/70-search.md`）。

---

## 复合组件索引（深度状态矩阵见对应 feature 文件）

| 组件 | 位置 | 解剖摘要 | 状态覆盖要点 | 深度规格 |
|---|---|---|---|---|
| `CommunityRail` | `layout/` | RailButton(DM/社区/新建) + UserMenu(头像+登出) | ✅ selected pill ✅ unread badge ⚠️ 无 loading ⚠️ 裸色值 | `features/20` |
| `ChannelList` | `layout/` | 顶部搜索框 + 频道列表/DM 列表(内联 `DMList`) | ✅ 多种空态 ✅ 搜索 loading ⚠️ 频道列表无骨架 | `features/30` `70` `40` |
| `MemberList` | `layout/` | ONLINE/OFFLINE 分组 + MemberRow(在线点) | ✅ 空态 ⚠️ 无 loading ❌ fetch 失败无反馈 | `features/60` |
| `ConnectionStatusBar` | `layout/` | 顶部条 | ✅ CONNECTING/RECONNECTING/DISCONNECTED | `features/50` |
| `ChannelView` | `chat/` | ChatHeader + MessageList + ChatInput + 可选 MemberList | ⚠️ 加载无骨架 ✅ 加载失败 toast | `features/40` |
| `DMChat` | `chat/` | ChatHeader + MessageList + ChatInput（无 MemberList） | ⚠️ 无加载/错误态 | `features/40` |
| `MessageList` | `chat/` | 滚动列表 + 顶部分页加载 | ✅ 空态「No messages yet」 ❌ 初始/历史加载无指示 ❌ 历史失败无反馈 ❌ 无「回到底部」 | `features/40` |
| `ChatInput` | `chat/` | InputGroup(附件+textarea+发送) + 文件预览 | ✅ 离线 disabled ✅ sending 态 ✅ 乐观插入 ⚠️ 上传无进度 ❌ 字符计数 | `features/40` `80` |
| `MessageBubble` | `chat/` | Avatar + 昵称(色编码) + 内容 + 附件 + 状态角标 | ✅ sending⏳/sent/failed✗ ⚠️ failed 重试无 handler | `features/40` |
| `ChatHeader` | `chat/` | 标题 + 在线点(DM) + toggle 成员列表 + 搜索 | ⚠️ peer 解析失败无反馈 | `features/40` `60` |
| `CreateCommunityDialog` | `communities/` | name 输入 + 创建 | ✅ submitting(disabled+文案) ✅ toast | `features/20` |
| `CreateChannelDialog` | `channels/` | name 输入 + 创建 | ✅ submitting ✅ toast | `features/30` |
| `LoginForm` / `RegisterForm` | `auth/` | 邮箱/密码(+用户名) | ✅ loading ✅ inline 错误 ⚠️ 无密码强度 | `features/10` |
| `AuthGuard` | `auth/` | 路由守卫 | ✅ loading 文本 ⚠️ 无骨架 | `features/10` |
| `SearchDialog`（死代码） | `search/` | Command 面板，多类结果 | — | `features/70`（复活决策） |

## 全局状态缺口汇总（跨组件，实现阶段统一补）

1. **加载态**：`MemberList`、`MessageList`(初始+历史)、`ChannelList`(频道列表)、`CommunityRail`(社区列表)、`ChannelView`/`DMChat` 均缺骨架/行内加载。→ 用 `Skeleton`（§7）。
2. **错误+重试**：`MemberList`、`MessageList` 历史、`ChatHeader` peer、搜索、presence 拉取均静默失败。→ 用统一 `empty-state`/错误条 + 重试按钮（`00` §8 新增原语）。
3. **死代码**：`SearchDialog.tsx`（复活为 ⌘K，见 `features/70`）、`components/dm/DMList.tsx`（删除，被 ChannelList 内联版取代）。
4. **裸色值**：见 `00` §6，全量替换为 token。
