# 01 · 信息架构（L0）

> 路由、导航模型、屏幕清单的唯一真相源。实现：`clients/web/src/App.tsx`、`components/layout/MainLayout.tsx`。

## 1. 路由表

实现：`App.tsx:16-31`。全局 `<ClientProvider>` + `<Toaster>` 包裹。

| 路径 | 组件 | 鉴权 | 说明 |
|---|---|---|---|
| `/login` | `LoginPage` | 否 | 登录 |
| `/register` | `RegisterPage` | 否 | 注册 |
| `/` | `Navigate → /@me` | 是 | 根重定向 |
| `/@me` | 占位 `<div>` | 是 | DM 主页：未选会话时显示「Select a conversation」 |
| `/@me/:peerId` | `DMChat` | 是 | 与 `:peerId` 的 DM 会话 |
| `/:communityId` | `ChannelView` | 是 | 社区根（无选中频道） |
| `/:communityId/:channelId` | `ChannelView` | 是 | 社区内某频道 |

- 鉴权由 `AuthGuard`（`components/auth/AuthGuard.tsx`）守卫：未登录 → 重定向 `/login`；`authStore.loading` 时显示加载态（见 `features/10-auth.md`）。
- `useAuthGate` 监听 SDK `unauthorized` 事件，token 失效时清状态 → 跳登录。

## 2. 导航模型（三栏 shell）

实现：`MainLayout.tsx:32-46`。已登录后固定三栏 + 顶部状态条：

```
┌──────────────────────────────────────────────────────────────┐
│ ConnectionStatusBar（仅 WS 非 CONNECTED 时可见）               │  ← 全局
├────────┬──────────────────┬───────────────────────────────────┤
│ Rail   │ ChannelList      │ Outlet（ChannelView / DMChat / 占位）│
│ 72px   │ 240px            │ flex-1                             │
│        │                  │                          ┌────────┐│
│        │                  │                          │Member  ││
│        │                  │                          │List    ││ ← 可选
│        │                  │                          │240px   ││
└────────┴──────────────────┴──────────────────────────┴────────┘
```

| 栏 | 组件 | 宽 | 职责 |
|---|---|---|---|
| 左 Rail | `CommunityRail` | 72px | DM 入口、社区图标(含未读角标)、新建社区、用户菜单/登出 |
| 中列 | `ChannelList` | 240px | 社区频道列表 **或** DM 列表（视 `view`）；顶部内联搜索 |
| 主区 | `Outlet` | flex-1 | `ChannelView` / `DMChat` / 占位 |
| 右列 | `MemberList` | 240px | 仅频道视图、`uiStore.showMemberList=true` 时挂载 |

- 中列内容由 `uiStore.view`（`'community' | 'dm'`）与路径共同决定：`/@me*` → DM 列表；`/:communityId*` → 频道列表。
- MemberList **默认隐藏**（`uiStore.showMemberList=false`，`uiStore.ts:22`），ChatHeader 的 toggle 按钮控制（见 `features/60-members.md` 的可发现性问题）。

## 3. App Map（屏幕清单 → spec 文件）

| 屏幕 | 路径 | 规格 |
|---|---|---|
| 登录 | `/login` | `features/10-auth.md` |
| 注册 | `/register` | `features/10-auth.md` |
| DM 主页（空） | `/@me` | `features/40-messaging.md`（DM 段） |
| DM 会话 | `/@me/:peerId` | `features/40-messaging.md`（DM 段） |
| 社区根（无频道） | `/:communityId` | `features/30-channels.md` |
| 频道会话 | `/:communityId/:channelId` | `features/40-messaging.md`（频道段） |
| 新建社区 Dialog | （Rail「+」） | `features/20-communities.md` |
| 新建频道 Dialog | （频道列表「+」） | `features/30-channels.md` |
| 搜索（内联/⌘K） | （中列顶部） | `features/70-search.md` |
| 用户菜单 | （Rail 头像） | `features/10-auth.md`（登出） |

## 4. 键盘快捷键

实现：`MainLayout.tsx:20-30`。

| 快捷键 | 行为 | 规格 |
|---|---|---|
| `⌘K` / `Ctrl+K` | 聚焦 ChannelList 搜索框 | `features/70-search.md` |
| `Enter`（输入框） | 发送消息 | `features/40-messaging.md` |
| `Shift+Enter` | 换行 | `features/40-messaging.md` |
| `Esc` | 关闭 Dialog / 搜索面板 | `02-component-library.md`（dialog） |

**【目标-待建】** 快捷键目前无「帮助」入口（`?` 召出清单）；见 future。

## 5. 初始化与数据流（全局）

`MainLayout` 挂载三个一次性的根 hook（`MainLayout.tsx:11-17`），决定全局状态可见性：

| Hook | 作用 | 暴露给 UI 的状态 |
|---|---|---|
| `useClientEvents` | SDK 事件 → Zustand | `uiStore.wsStatus`、unread 自增、presence 增减 |
| `useInitialData` | 登录后拉首屏（社区/频道/未读/presence） | **无全局 loading 标志**（缺口，见下） |
| `useMessageSync` | 回补丢失消息（重连/可见性/30s 轮询） | **无 sync 标志**（缺口） |

**全局缺口（影响所有屏幕）**：
1. **【实现】无「首屏加载」shell 标志**：`useInitialData` 用 `loaded.current` ref（内部），UI 拿不到「正在加载首屏」信号 → 社区/频道列表在数据到达前直接显示空态，与「真的没有」无法区分。**目标态**：`useInitialData` 暴露 `initialLoading`，shell 在加载中显示骨架（见 `02-component-library.md` skeleton 用法）。
2. **【实现】无「回补中」标志**：`useMessageSync` 失败只 `console.warn`，UI 无感知。**目标态**：可选「正在同步消息」微指示（非阻塞）。

## 6. 深链与回退

- 频道/DM 均为可深链 URL（`:communityId/:channelId`、`/@me/:peerId`），未授权访问的行为见 `features/30-channels.md`（非成员读频道）、`features/60-members.md`。
- 浏览器后退遵循路由历史；Dialog/Popover 不入栈，`Esc`/外点关闭。
