# 00 · 设计系统（L1）

> 本文件是 Constell Web 客户端的视觉语言唯一真相源。所有屏幕与组件（`docs/design/02-component-library.md`、`features/*`）以此为准。
> 色板基于 **Catppuccin Mocha**（仅暗色，无 light 变体）。实现位于 `clients/web/src/index.css`。

## 1. 设计原则

1. **暗色优先，单一主题**：只维护一套暗色（Catppuccin Mocha），不引入 light 变体，降低状态爆炸。
2. **token 优先，禁止裸十六进制**：组件**必须**使用语义 token（`bg-background`、`text-muted-foreground`…），不得写死 `#1e1e2e` 之类的色值。例外见 §6。
3. **状态可感知**：任何交互面都要有 rest/hover/focus/active/disabled 的视觉区分；任何异步都要有 loading/success/error 反馈（见各 feature 的状态矩阵）。
4. **一致性优先于创意**：复用 `components/ui/*` 原语，不另造同类组件。

## 2. 色板（Catppuccin Mocha → 语义 token）

实现：`clients/web/src/index.css:52-86`。下表给出 token → HSL → Catppuccin 名称 → 用途。

| Token (`--*`) | 值 (hsl) | Catppuccin | 用途 |
|---|---|---|---|
| `--background` | `30 10% 11%` | Base | 应用底色 |
| `--foreground` | `267 84% 94%` | Text | 主文字 |
| `--card` | `30 10% 14%` | Mantle | 卡片/弹层底 |
| `--card-foreground` | `267 84% 94%` | Text | 卡片文字 |
| `--popover` | `30 10% 14%` | Mantle | Dialog/Popover 底 |
| `--popover-foreground` | `267 84% 94%` | Text | 弹层文字 |
| `--primary` | `263 70% 58%` | Mauve | 主操作/强调 |
| `--primary-foreground` | `0 0% 100%` | — | 主操作上的文字 |
| `--secondary` | `267 11% 25%` | Surface1 | 次级容器 |
| `--secondary-foreground` | `267 84% 94%` | Text | — |
| `--muted` | `267 11% 25%` | Surface1 | hover 底/静默块 |
| `--muted-foreground` | `267 11% 52%` | Subtext0 | 次要文字/占位符 |
| `--accent` | `267 11% 25%` | Surface1 | 选中/hover 强调底 |
| `--accent-foreground` | `267 84% 94%` | Text | — |
| `--destructive` | `0 72% 61%` | Red | 错误/删除/危险 |
| `--destructive-foreground` | `0 0% 100%` | — | — |
| `--border` | `267 11% 20%` | Surface0 | 分隔线/边框 |
| `--input` | `267 11% 20%` | Surface0 | 输入框边框 |
| `--ring` | `263 70% 58%` | Mauve | focus 环 |
| `--sidebar` | `30 10% 11%` | Base | 侧栏底 |
| `--sidebar-*` | （见 css） | — | 侧栏全套语义色 |

**语义别名**（`@theme inline` 映射，`index.css:8-49`）：`bg-background`、`text-foreground`、`bg-card`、`bg-popover`、`bg-primary`/`text-primary-foreground`、`bg-secondary`、`bg-muted`/`text-muted-foreground`、`bg-accent`、`text-destructive`、`border-border`、`ring-ring` 等。Tailwind 类直接用这些别名。

**状态色约定**（用于在线指示、未读、成功等，复用 Catppuccin 语义）：

| 含义 | 推荐 token / 值 | 备注 |
|---|---|---|
| 在线 (online) | Green `hsl(115 54% 76%)`（Catppuccin Green） | presence 点 |
| 离线 (offline) | `text-muted-foreground` | — |
| 未读角标 | `--destructive`（Red） | 当前实现用红；与 Discord 一致 |
| 连接中 (connecting) | Yellow `hsl(41 86% 83%)`（Catppuccin Yellow） | ConnectionStatusBar |
| 已断开 (disconnected) | `--destructive` | ConnectionStatusBar |
| 成功 (success) | Green `hsl(115 54% 76%)` | toast 成功 |
| 错误 (error) | `--destructive` | toast/inline错误 |

## 3. 字体

- 字族：`'Geist Variable', sans-serif`（`index.css:10`，经 `@fontsource-variable/geist` 引入）。
- `--font-heading` = `--font-sans`（标题与正文同字族，靠字重/字号区分）。
- 字号阶（建议固定为一组 utility 约定，避免散落 `text-[13px]`）：

| 用途 | class | 备注 |
|---|---|---|
| 页/屏标题 | `text-base font-medium` | Dialog 标题等 |
| 正文/消息体 | `text-sm` | 默认 |
| 次要文字 | `text-xs text-muted-foreground` | 时间戳、副标题 |
| 角标 | `text-[10px] font-bold` | 未读数 |

## 4. 间距 / 圆角

- 基础 `--radius: 0.5rem`（`index.css:77`），派生阶（`index.css:42-48`）：
  `sm=0.6×`、`md=0.8×`、`lg=1×`、`xl=1.4×`、`2xl=1.8×`、`3xl=2.2×`、`4xl=2.6×`。
- 用 Tailwind 圆角类 `rounded-lg`/`rounded-xl`/`rounded-2xl`，避免裸 `rounded-[16px]`。
- 间距用 Tailwind 4/6/8 步进；列宽固定：CommunityRail `w-[72px]`、ChannelList `w-[240px]`、MemberList `w-[240px]`。

## 5. 动效 / 阴影 / 图标

- 动效库：`tw-animate-css`（`index.css:2`）。Dialog/弹层用 `data-open:animate-in fade-in-0 zoom-in-95`。
- 过渡统一 `transition-all duration-200`（Rail 按钮 morph、选中 pill）。
- 阴影：弹层 `shadow-xl`；避免重投影。
- 图标：`lucide-react`（`^1.17.0`）。统一 `size-4`（按钮内）、`size-3.5`（密集列表内）。

## 6. 规范缺口（必须修正，对应实现缺口标【实现】）

> 这些是"token 已定义但实现没遵守"的不一致，必须在实现阶段消除。feature 文件的"当前实现缺口"会逐条引用。

1. **【实现】CommunityRail 使用裸十六进制**：`CommunityRail.tsx` 通篇用 `#11111b`/`#313244`/`#cdd6f4`/`#7c3aed`/`#f38ba8`/`#585b70`/`#1e1e2e`，而非 `bg-sidebar`/`bg-muted`/`text-foreground`/`bg-primary`/`bg-destructive`/`text-muted-foreground`/`bg-popover`。**修正**：全部改成语义 token 类。参见 `features/20-communities.md`。
2. **【实现】ConnectionStatusBar / UserMenu 等同样存在裸色值**：实现阶段统一排查 `grep -rn "#[0-9a-fA-F]\{6\}" clients/web/src/components` 并替换为 token。
3. **【实现】"未读角标用红"** 是当前实现（`CommunityRail.tsx:234` 用 `#f38ba8`=Red）；本规范确认采用红色未读角标（与 Discord 一致），不改为 primary。

## 7. 反馈机制（全局）

- **Toast**：`sonner`，已在 `App.tsx:14` 全局挂载 `<Toaster position="top-center" richColors />`。
  - 用途：操作成功/失败的非阻塞反馈（创建社区、发消息失败、加入失败…）。
  - 原则：**瞬态错误用 toast；阻塞/需决策的错误用 inline 或 Dialog**（见各 feature）。
- **确认对话框**：危险操作（删除、踢出、离开）**必须**走 Dialog 二次确认（当前缺失，见 `features/60-members.md`、`40-messaging.md` 的【目标-待建】）。

## 8. 原语清单（由 shadcn over @base-ui/react 提供）

来源：`clients/web/src/components/ui/*`，底层 `@base-ui/react`（非 Radix）。完整解剖见 `02-component-library.md`。

`button` · `input` · `textarea` · `dialog` · `avatar` · `badge` · `scroll-area` · `skeleton` · `tooltip` · `separator` · `input-group` · `command`(cmdk)

**缺失原语（需新增，对应【目标-待建】）**：

| 原语 | 用途 | 驱动的 feature |
|---|---|---|
| `dropdown-menu` / `context-menu` | 消息右键（编辑/删除）、频道/成员右键 | 40 / 30 / 60 |
| `spinner` | 行内加载（区别于骨架） | 全局 |
| `empty-state` | 统一空态（图标+文案+可选动作） | 全局 |
| `alert-dialog` | 危险确认 | 40 / 60 |
| `popover` | 头像悬停资料卡 | 60 |
| `tabs` / `switch` | 设置（future） | future |

> 新增原语统一放 `components/ui/`，遵循 cva + `data-slot` 既有模式。
