# 70 · 搜索（内联过滤 + ⌘K 全局面板）

> 域前缀 `SEARCH-*`。实现：`components/layout/ChannelList.tsx`(内联搜索 + SearchResultsList)、`components/search/SearchDialog.tsx`(**死代码**，待复活为 ⌘K)、`hooks/usePullPresence.ts`(结果里的 presence)。

## 0. 概述与用户目标

两种互补的搜索：
- **内联过滤**（中列搜索框，键入即生效）：在当前频道/DM 列表里按名过滤。
- **全局搜索**（⌘K 面板，目标态）：跨社区/用户/消息/DM 全局检索，结果可跳转/加入。

用户目标：快速找到「某条消息/某个人/某个社区」。

## 1. 屏幕与布局

### 现状（内联双模）
中列顶部搜索框：键入→本地过滤频道/DM；按 Enter→调 `/search` API，内联展开 SearchResultsList（Communities/Users/Messages/DM 四组）。

### 目标态（⌘K 面板）
- 搜索框保留为「当前列内过滤」（频道/DM by name）。
- `⌘K` 打开**全局** Command 面板（复用已存在的 `SearchDialog.tsx`）：防抖搜索、骨架、相关性%、分组结果、Esc 关闭。
- ChatHeader 的搜索按钮改为打开 ⌘K 面板（去掉 `document.querySelector` hack）。

```
目标态：
中列 [🔍 Filter channels…]      ← 键入：本地过滤当前列
⌘K → ┌─ Search ──────────────┐
      │ [全局搜索___________] │
      │ Communities (3)       │
      │ Users (2)  · presence │
      │ Messages (5)          │
      │ Direct Messages (1)   │
      └───────────────────────┘
```

## 2. 状态矩阵

### 内联过滤

| 状态 | 触发 | 期望 UI | 断言 |
|---|---|---|---|
| 过滤中 | 键入 | 列表按 name 实时收窄 | 仅匹配项 |
| 无匹配(频道) | 过滤无结果 | 「No matching channels」 | 文案 |
| 无匹配(DM) | 过滤无结果 | 「No matching conversations」 | 文案 |

### 全局搜索（⌘K 面板 / 现状 Enter）

| 状态 | 触发 | 期望 UI | 断言 |
|---|---|---|---|
| 空查询 | 打开未输入 | 「Start typing to search…」+ Esc 提示 | 文案 |
| 搜索中 | 防抖/请求中 | 骨架行 / 「Searching...」 | 骨架可见 |
| 有结果 | 返回≥1 | 分组：Communities/Users/Messages/DM | 各组可见 |
| 无结果 | 返回空 | 「No results found」 | 文案 |
| 社区-未加入 | 结果 joined=false | 「Join」按钮 | Join 可点 |
| 社区-已加入 | 结果 joined=true | 「Joined」；行点击进入 | 进入社区 |
| 失败 | 请求抛错 | 错误提示 + 重试（**当前静默**） | 错误可见 |

## 3. 流程

- **内联过滤**：键入→`filteredChannels`/DM filter（纯前端，无请求）。
- **全局搜索（现状 Enter）**：Enter→`client.search(q,{limit:10})`→SearchResultsList 渲染；用户/DM 项拉 presence。
- **目标（⌘K）**：`⌘K`→打开 Command 面板→防抖 300ms→`search`→分组结果→选择项跳转/加入→关闭。
- **结果动作**：社区「Join」→`joinCommunity`→写 store→进入；用户/DM→跳 `/@me/:id`；消息→跳 `/:cid/:chid`。

## 4. 验收标准

```
AC SEARCH-FILTER-1  内联过滤频道
  GIVEN 频道 [general, random, dev]
  WHEN 在中列搜索框输入 "ra"
  THEN 仅显示 #random（无 API 请求）

AC SEARCH-GLOBAL-1  ⌘K 打开全局面板（目标-待建，当前红）
  GIVEN 已登录
  WHEN 按 ⌘K（或 Ctrl+K）
  THEN 打开全局搜索面板，输入自动聚焦

AC SEARCH-GLOBAL-2  跨类结果
  GIVEN 查询命中多类
  WHEN 输入查询并防抖结束
  THEN Communities/Users/Messages/DM 分组显示

AC SEARCH-GLOBAL-3  空查询引导
  GIVEN 面板已开、未输入
  THEN 显示「Start typing to search…」+ Esc 关闭提示

AC SEARCH-GLOBAL-4  无结果
  GIVEN 查询无任何命中
  THEN 显示「No results found」

AC SEARCH-GLOBAL-5  搜索失败可见（当前红 — 缺口 3）
  GIVEN /search 请求失败
  THEN 显示错误 + 重试（非静默）

AC SEARCH-GLOBAL-6  结果跳转
  GIVEN 命中某频道消息
  WHEN 点击该结果
  THEN 跳转到对应频道并高亮/显示该消息

AC SEARCH-GLOBAL-7  Esc 关闭
  GIVEN 面板已开
  WHEN 按 Esc
  THEN 关闭面板，焦点归位

AC SEARCH-JOIN-1  从结果加入（引用 COMM-JOIN-1）
  GIVEN 结果含未加入公开社区
  WHEN 点 Join
  THEN 加入并进入（rail 立即更新）
```

## 5. 边界与约束

- 全局搜索 `limit=10`（`ChannelList.tsx:74`）。
- 防抖 300ms（`SearchDialog.tsx` 现有实现）。
- 索引异步：新发消息入索引有延迟（见 e2e `waitForTimeout`）。
- 社区搜索仅 `is_public` 的可被发现（见记忆 [[community-search-discovery]]）。

## 6. 当前实现缺口

1. **【实现·死代码】`SearchDialog.tsx`（481 行）零引用** → **已复活为 ⌘K 全局面板**：挂载到 `MainLayout`，`⌘K`/Ctrl+K 切换，ChatHeader 搜索按钮经 `open-search` 事件打开；清掉 `searchInputRef` 死代码。✅ AC SEARCH-GLOBAL-1 / SEARCH-GLOBAL-3（空查询引导随面板可用）。
2. **【实现】内联 Enter 搜索静默失败**：~~`ChannelList.tsx:76` `.catch(()=>setSearchResults(null))`~~ → **已修**：`searchError` 状态 + 内联 `ErrorState`（Retry 重跑 `runSearch`）。✅ AC SEARCH-GLOBAL-5。
3. **【实现】ChatHeader 搜索按钮 hack**：~~`document.querySelector('[placeholder="Search..."]')`~~ → **已修**：改为 `window.dispatchEvent(new CustomEvent('open-search'))` 打开 ⌘K 面板。
4. **【实现】裸色值**：ChannelList/SearchDialog 通篇 hex。改 token。
5. **【目标-待建】搜索历史 / 最近结果** —— 列 future。
6. **【目标-待建】消息结果无高亮跳转锚点**（当前只跳频道，不定位到具体消息）。→ AC SEARCH-GLOBAL-6 需后端/前端锚点支持。

## 7. 待定问题

- ⌘K 面板是否包含「跳转到频道/DM」的快速导航（类 Spotlight）？建议含（Command 面板天然支持）。
- 搜索结果分页（limit>10）是否需要？暂限 10。
