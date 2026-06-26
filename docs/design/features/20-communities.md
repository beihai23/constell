# 20 · 社区（创建 / 发现 / 加入 / 切换 / 离开）

> 域前缀 `COMM-*`。实现：`components/layout/CommunityRail.tsx`、`components/communities/CreateCommunityDialog.tsx`、`components/layout/ChannelList.tsx`(搜索结果里的社区项 + Join)、`hooks/useInitialData.ts`。

## 0. 概述与用户目标

创建社区（自己成 owner）、发现并加入公开社区、在多个社区间切换、离开社区。用户目标：一站式管理「我所在的社区」。

## 1. 屏幕与布局

- **Rail（左 72px）**：DM 入口 → 分隔 → 社区图标列表(含未读角标) →「+」新建 → 底部用户菜单。
- **新建社区 Dialog**：name(必填,≤100) + description(可选,≤500) → Create/Cancel。
- **发现/加入**：经搜索（`features/70`）的「Communities」结果项，未加入显示「Join」按钮。

```
┌─ Rail ─┐   ┌─ CreateCommunityDialog ─┐
│ 💬 DM  │   │ Create a Community       │
│ ───── │   │  Community Name [____]   │
│ [G]    │   │  Description(optional)   │
│ [C]•2  │   │  [____]                  │
│ [+]    │   │  [Cancel] [Create]       │
│ ...    │   └──────────────────────────┘
│ [avatar]│
└────────┘
```

## 2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| Rail-有社区 | 已加入≥1 | 图标列表；选中 pill；未读角标 | `communities` Map | 各社区图标可见 |
| Rail-空 | 未加入任何 | 仅 DM +「+」（无社区图标） | `communities.size=0` | 无社区图标 |
| Rail-加载 | 首屏拉取中 | （目标）骨架图标；当前直接空 | `initialLoading` | 骨架 |
| 选中社区 | 点击图标 / 路由变化 | 左侧白 pill + 方形选中态 | `currentCommunityId` | pill 可见 |
| 未读聚合 | 社区内任一频道有未读 | 图标右上红角标(聚合数,>99 显 99+) | 聚合 `channelUnreads` | 角标计数 |
| 创建-提交中 | 点 Create | Create 按钮 disabled +「Creating…」 | `submitting=true` | 按钮 disabled |
| 创建-成功 | API 成功 | toast「Created "X"」；关闭弹窗；rail 立即出现新社区；跳进默认频道 | `addCommunity` + `setChannels` | rail 有新社区；URL 进频道 |
| 创建-失败 | API 抛错 | toast「Failed to create community」；弹窗保留 | — | toast 可见 |
| 发现-未加入 | 搜索返回 `joined=false` | 「Join」按钮 | — | Join 可见可点 |
| 加入-成功 | 点 Join | rail 立即出现该社区；跳进 | `addCommunity`+`setChannels` | rail 有；URL 进社区 |
| 加入-失败 | join 抛错 | toast「Failed to join community」 | — | toast |
| 离开 | （目标）成员主动离开 | alert-dialog 确认 → rail 移除 | — | rail 移除 |

## 3. 流程

- **创建**：Rail「+」→ Dialog → 填 name(+desc) → `createCommunity` → `addCommunity` + 拉 channels → toast → 跳 `/:communityId/:firstChannelId`。
- **发现+加入**：搜索框 Enter →「Communities」结果 → 点「Join」→ `joinCommunity` → 写 store → 跳 `/:communityId`（**行点击只进已加入社区，不会误加入**，`ChannelList.tsx:247-252`）。
- **切换**：点 rail 图标 → `navigate('/:communityId')`。
- **离开**：【目标-待建】当前无 UI（API 是否存在待查）。

## 4. 验收标准

```
AC COMM-CREATE-1  创建社区并进入默认频道
  GIVEN 已登录用户
  WHEN 点 Rail「+」→ 填 name → Create
  THEN toast 成功；rail 出现新社区；URL 落到 /<id>/<general频道id>

AC COMM-CREATE-2  创建失败有反馈
  GIVEN 创建请求失败
  THEN toast「Failed to create community」，弹窗不关

AC COMM-RAIL-1  未读聚合角标
  GIVEN 社区X 的 #a 有 1 未读、#b 有 2 未读
  THEN 社区X 图标角标=3

AC COMM-RAIL-2  选中态可视
  GIVEN 当前在某社区
  THEN 该社区 rail 图标左侧有白色 pill

AC COMM-JOIN-1  从搜索加入公开社区（引用 SEARCH-*）
  GIVEN 搜索到未加入的公开社区
  WHEN 点 Join
  THEN rail 立即出现该社区，无需 reload

AC COMM-JOIN-2  行点击不误加入
  GIVEN 搜索到未加入的社区
  WHEN 点击行体（非 Join 按钮）
  THEN 不发起 join（仅已加入社区才会进入）

AC COMM-LEAVE-1  离开社区（目标-待建，当前红）
  GIVEN 已加入某非 owner 社区
  WHEN 在该社区触发「Leave」并确认
  THEN rail 移除该社区
```

## 5. 边界与约束

- name ≤100、description ≤500（`CreateCommunityDialog.tsx:130,150`）。
- 后端建社区时自动 seed「general」频道（见 `communitiesStore` + commit `e2beaf4`）。
- 私有社区：当前无创建私有社区的入口（见 future / 记忆 [[community-search-discovery]]）。

## 6. 当前实现缺口

1. **【实现】Rail 裸色值**：`CommunityRail.tsx` 全程 `#11111b/#313244/#cdd6f4/#7c3aed/#f38ba8/#585b70/#1e1e2e`。**修正**：改 token（`bg-sidebar`/`bg-muted`/`text-foreground`/`bg-primary`/`bg-destructive`/`text-muted-foreground`/`bg-popover`）。→ `00` §6。
2. **【实现】Rail 无加载骨架**：社区列表在 `useInitialData` 返回前为空，与「真无社区」无法区分。→ AC Rail-加载；依赖 `01` §5 全局 `initialLoading`。
3. **【实现】Create Dialog 裸色值**：`CreateCommunityDialog.tsx:106-170` 通篇 hex。**修正**：用 `bg-popover`/`text-popover-foreground`/`bg-input` 等。
4. **【目标-已建】无离开社区 UI** → **已建**：api-gateway 新增 `DELETE /communities/{id}/leave`（→ `LeaveCommunity` RPC，owner 被服务端拒），SDK `leaveCommunity`，ChannelList 头部「Leave」按钮 + 确认 Dialog → `removeCommunity` + 跳 `@me`。✅ AC COMM-LEAVE-1。
5. **【目标-待建】无社区设置**（改名/改描述/删社区）—— 列 future。
6. **【实现】未读角标用裸 `<span>`**（`CommunityRail.tsx:234`），应用 `Badge`（见 `02` §6）。

## 7. 待定问题

- 离开/删除社区的后端 RPC 是否就绪？（影响 COMM-LEAVE-1 能否落地）
- 私有社区 + 邀请加入：是否纳入本期？（建议 future）
