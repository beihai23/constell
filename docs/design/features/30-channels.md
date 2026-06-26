# 30 · 频道（创建 / 列表 / 选中）

> 域前缀 `CHAN-*`。实现：`components/layout/ChannelList.tsx`(频道分区)、`components/channels/CreateChannelDialog.tsx`、`components/chat/ChannelView.tsx`(getChannels)。

## 0. 概述与用户目标

在社区内创建文字频道、浏览频道列表、进入频道。用户目标：快速找到/建立要发言的频道。

## 1. 屏幕与布局

中列（240px）顶部：标题（社区名）+ 内联搜索框；下方「TEXT CHANNELS」标题 +「+」新建；频道项列表。

```
┌─ ChannelList ────────┐
│ Gophers United        │   ← 社区名 / "Select a Community"
│ [🔍 Search...       ] │   ← 本地过滤 + Enter 全局搜索(见 70)
│ TEXT CHANNELS    [+]  │
│  # general            │   ← 选中: 方形高亮
│  # random        3    │   ← 未读角标
│  # dev                │
└───────────────────────┘
```

## 2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| 有频道 | 社区有频道 | 「TEXT CHANNELS」+ 频道项列表 | `channels.get(cid)[]` | 频道可见 |
| 选中频道 | 点击 / 路由匹配 | 该项方形高亮 `bg-muted` | `currentChannelId` | 选中样式 |
| 未读 | 该频道有未读 | `#name` 加粗 + 右红角标 | `channelUnreads` | 角标 |
| 本地过滤-无匹配 | 输入过滤无结果 | 「No matching channels」 | `filtered.length=0 && list>0` | 文案 |
| 无频道 | 社区无任何频道 | 「No channels yet」 | `channelList.length=0` | 文案 |
| 未选社区 | 路由非社区/DM | 「Select a community or open DMs」 | 无 `communityId` | 文案 |
| 列表加载 | getChannels 进行中 | （目标）骨架行 | loading | 骨架 |
| 创建-提交中 | 点 Create | Create disabled +「Creating…」 | `submitting` | 按钮 disabled |
| 创建-成功 | API 成功 | toast「Created #x」；列表立即出现；跳进新频道 | `addChannel` | 新频道可见；URL 进 |
| 创建-失败 | API 抛错 | toast「Failed to create channel」 | — | toast |

## 3. 流程

- **列出**：进入社区 → `ChannelView` 调 `getChannels` → `setChannels` → ChannelList 渲染（首屏数据也可能由 `useInitialData` 预取）。
- **选中**：点击频道项 → `navigate('/:communityId/:channelId')`。
- **本地过滤**：在搜索框键入 → `filteredChannels` 按 name 包含过滤（不触发 API）。
- **创建**：「+」→ Dialog → name → `createChannel` → `addChannel` → toast → 跳进新频道。
- **全局搜索**：Enter → 见 `features/70`。

## 4. 验收标准

```
AC CHAN-LIST-1  频道列表显示
  GIVEN 用户进入已加入的社区
  THEN 中列显示该社区所有频道（含 seed 的 general）

AC CHAN-LIST-2  选中态与未读角标
  GIVEN #random 有未读
  WHEN 用户打开 #general
  THEN #general 高亮选中；#random 显示未读角标

AC CHAN-LIST-3  本地过滤
  GIVEN 频道 [general, random, dev]
  WHEN 在搜索框输入 "ra"
  THEN 仅显示 #random（不触发 API）

AC CHAN-LIST-4  无频道空态
  GIVEN 社区无频道（极少，因 seed general）
  THEN 显示「No channels yet」

AC CHAN-CREATE-1  创建频道并进入
  GIVEN owner 在某社区
  WHEN 点「+」→ 填 name → Create
  THEN toast 成功；列表立即出现新频道；URL 进新频道

AC CHAN-CREATE-2  名称长度约束
  GIVEN name 超过 50 字符
  THEN 输入框不再接受输入（maxLength=50）

AC CHAN-CREATE-3  创建失败有反馈
  GIVEN 创建请求失败
  THEN toast「Failed to create channel」

AC CHAN-VISIT-1  非成员访问私有频道（引用 60 边界）
  GIVEN 非成员直接打开 /:cid/:chid
  THEN 不显示历史消息（见 features/60 / 40 B）
```

## 5. 边界与约束

- name ≤50（`CreateChannelDialog.tsx:105`）。
- 后端建社区时 seed「general」；故「无频道」空态在实际中很少触发。
- 频道类型：当前仅文本频道（无语音/分类分组）。

## 6. 当前实现缺口

1. **【实现】ChannelList 裸色值**：通篇 `#181825/#cdd6f4/#585b70/#11111b/#313244/#1e1e2e/#a6adc8/#a6e3a1/#f38ba8/#cba6f7/#45475a`。**修正**：改 token。
2. **【实现】无频道列表加载骨架**：依赖 `getChannels` 返回前列表为空 → 可能闪现「No channels yet」。→ AC CHAN-LIST-1（加载态）。
3. **【实现】CreateChannelDialog 裸色值**：`CreateChannelDialog.tsx:81-129`。改 token。
4. **【目标-待建】无频道设置**（改名/删频道/话题）—— 列 future。
5. **【目标-待建】无频道分类分组**（Discord 的分类）—— 列 future。
6. **【实现】未读角标裸 `<span>`**，应用 `Badge`。

## 7. 待定问题

- 频道 rename/delete 的后端 RPC？（影响设置项能否落地）
- 私有频道（频道级可见性）是否需要？（当前权限在成员级，见 `features/60`）— 建议 future。
