# Constell — Plan 5: Web 客户端设计规格

> **阶段定位：** Plan 5 在 Plan 4 (File + Search + Notify) 之后，为 Constell 构建 Web 前端客户端和 SDK-JS 库。本文档是 Plan 5 实施的唯一上下文源。

## 决策摘要

| 决策点 | 结论 |
|--------|------|
| 框架 | React 18/19 + TypeScript + Vite |
| UI 组件库 | shadcn/ui + Tailwind CSS |
| 状态管理 | Zustand |
| 结构 | 独立 SDK-JS 包 (`sdk/sdk-js/`) + 独立 React 应用 (`clients/web/`) |
| SDK 模式 | 胖 SDK——封装所有后端交互（REST + WS + Protobuf） |
| Protobuf | @bufbuild/protobuf (protobuf-es)，Buf 生成 TS 代码 |
| 主题 | 仅深色 (MVP)，Catppuccin Mocha 风格 |
| 用户术语 | Community（不使用 "Server"） |
| 部署 | Nginx 反代 + Docker Compose `web` 服务 (port 3000) |

## 产出物

| 产出物 | 位置 | 说明 |
|--------|------|------|
| SDK-JS | `sdk/sdk-js/` | npm 包，封装 REST + WS + Protobuf，零 DOM 依赖 |
| Web 应用 | `clients/web/` | React SPA，Discord 风格三栏 IM 布局 |
| Proto TS 代码 | `sdk/sdk-js/src/protobuf/` | `buf generate` 生成的 gateway/v1 TS 类型 |
| Nginx 配置 | `clients/web/nginx.conf` | SPA fallback + REST/WS 反代 |
| Docker 配置 | `clients/web/Dockerfile` + `docker-compose.yml` 更新 | 多阶段构建 + Compose 集成 |

## MVP 功能范围

### 核心（必须有）

- 登录 / 注册页
- Community/Channel 侧边栏（加入的 Community 列表 → Channel 列表）
- 频道消息列表（实时收发）
- DM 视图（好友列表 → DM 对话）
- WebSocket 实时消息推送
- 断线重连 + 连接状态提示

### 重要（体验关键）

- 文件上传 UI（发送图片/文件附件）
- 搜索 UI（用户/消息搜索）
- 未读标记（频道/DM 红点）
- 用户在线状态显示
- 消息发送状态（sending → sent / failed + 重发）

### 可延后

- Community 设置/管理页（创建/编辑/删除）
- 角色权限管理 UI
- 用户 Profile 编辑页
- 频道管理（创建/编辑/排序）
- 响应式移动端适配
- 浅色主题

---

## 1. SDK-JS 设计

### 1.1 目录结构

```
sdk/sdk-js/
├── src/
│   ├── client.ts          # ConstellClient 主入口
│   ├── auth.ts            # AuthManager — JWT 管理
│   ├── ws-manager.ts      # WSManager — WebSocket 连接管理
│   ├── rest-client.ts     # RESTClient — HTTP API 调用
│   ├── event-bus.ts       # EventBus — 类型安全事件分发
│   ├── protobuf/          # buf generate 生成的 TS 代码
│   │   └── gateway/v1/
│   │       └── gateway_pb.ts
│   ├── codec.ts           # 二进制帧编解码
│   └── types.ts           # 公共类型定义
├── package.json
├── tsconfig.json
├── buf.gen.yaml           # Buf JS 代码生成配置
└── vitest.config.ts
```

### 1.2 ConstellClient — 主入口

```typescript
class ConstellClient {
  private auth: AuthManager;
  private ws: WSManager;
  private rest: RESTClient;
  private bus: EventBus;

  constructor(config: { apiUrl: string; wsUrl: string });

  // 连接管理
  connect(): void;
  disconnect(): void;
  get status(): 'connected' | 'connecting' | 'disconnected';

  // 认证
  login(email: string, password: string): Promise<User>;
  register(username: string, email: string, password: string): Promise<User>;
  logout(): void;

  // DM
  sendDM(receiverId: string, content: string, fileIds?: string[]): Promise<void>;
  getDMHistory(peerId: string, opts?: { limit?: number; cursor?: string }): Promise<{ messages: DMMessage[]; nextCursor?: string }>;
  getDMConversations(opts?: { limit?: number; cursor?: string }): Promise<{ conversations: DMConversation[]; nextCursor?: string }>;

  // Channel Messages
  sendChannelMessage(channelId: string, content: string, fileIds?: string[]): Promise<void>;
  getChannelHistory(channelId: string, opts?: { limit?: number; cursor?: string }): Promise<{ messages: ChannelMessage[]; nextCursor?: string }>;
  subscribeChannel(channelId: string): void;
  unsubscribeChannel(channelId: string): void;

  // Community
  listCommunities(): Promise<Community[]>;
  createCommunity(name: string, description?: string): Promise<Community>;
  getChannels(communityId: string): Promise<Channel[]>;
  getMembers(communityId: string, opts?: { limit?: number; cursor?: string }): Promise<{ members: Member[]; nextCursor?: string }>;
  addMember(communityId: string, userId: string): Promise<void>;

  // User
  getUser(userId: string): Promise<User>;
  listFriends(opts?: { limit?: number; cursor?: string }): Promise<{ friends: User[]; nextCursor?: string }>;

  // File（底层接口接受 Uint8Array，零 DOM 依赖；React 应用层封装 File/Blob 便利方法）
  uploadFile(data: Uint8Array, filename: string, contentType: string): Promise<FileInfo>;
  getFileURL(fileId: string): Promise<string>;

  // Search
  search(query: string, opts?: { types?: SearchType[]; limit?: number }): Promise<SearchResults>;

  // Notify
  getUnreadCounts(): Promise<UnreadCounts>;
  markDMRead(conversationId: string): Promise<void>;
  markChannelRead(channelId: string): Promise<void>;

  // 事件订阅
  on(event: ClientEvents, handler: Function): void;
  off(event: ClientEvents, handler: Function): void;
  removeAllListeners(): void;
}
```

### 1.3 事件类型

| 事件名 | 载荷 | 触发时机 |
|--------|------|----------|
| `connected` | — | WebSocket 连接成功 |
| `disconnected` | — | WebSocket 断开 |
| `reconnecting` | `{ attempt: number }` | 开始重连 |
| `reconnected` | — | 重连成功 |
| `dm_received` | `DMMessage` | 收到 DM 消息 |
| `channel_message` | `ChannelMessage` | 收到频道消息 |
| `user_online` | `{ userId: string }` | 用户上线 |
| `user_offline` | `{ userId: string }` | 用户下线 |
| `notification` | `NotificationEvent` | 通知推送（未读更新） |
| `message_sending` | `TempMessage` | 本地消息发送中（乐观更新） |
| `message_sent` | `TempMessage` | 本地消息已确认 |
| `message_failed` | `TempMessage` | 本地消息发送失败 |

### 1.4 AuthManager

```typescript
class AuthManager {
  private accessToken: string | null;
  private refreshToken: string | null;
  private refreshPromise: Promise<void> | null;

  // 存储位置：localStorage
  // key: constell_access_token / constell_refresh_token

  async getValidToken(): Promise<string>;
  // 如果 accessToken 过期：
  //   1. 如果已有 refreshPromise 在执行，await 它（防并发）
  //   2. 否则发 POST /api/v1/auth/refresh
  //   3. 更新两个 token 到 localStorage
  //   4. 如果 refresh 也失败，触发 logout

  async login(email: string, password: string): Promise<User>;
  async register(username: string, email: string, password: string): Promise<User>;
  logout(): void;
  initFromStorage(): User | null;
}
```

### 1.5 WSManager

**状态机：**

```
DISCONNECTED → connect() → CONNECTING → ws.onopen → CONNECTED
CONNECTED → ws.onclose/error → RECONNECTING → delay → connect() → CONNECTING
RECONNECTING → max retries → DISCONNECTED (仅 disconnect() 调用时)
CONNECTED → disconnect() → DISCONNECTED
```

**重连参数：**

| 参数 | 值 |
|------|-----|
| baseDelay | 1s |
| maxDelay | 30s |
| maxRetries | Infinity（持续重连直到手动断开） |
| jitter | ±20% |

**重连延迟序列：** 1s → 2s → 4s → 8s → 16s → 30s → 30s → ...

**心跳：** 每 30s 发送 `HEARTBEAT` ClientMessage，与后端 WS Gateway 心跳间隔匹配。

**二进制帧：** `ws.binaryType = 'arraybuffer'`，使用 length-prefixed protobuf（4 字节 big-endian 长度 + protobuf payload），与后端 Protocol 匹配。

**重连时的关键动作：**
1. 调用 `authManager.getValidToken()` 确保 JWT 有效
2. 用新 token 建立 WebSocket 连接：`ws://host/ws?token={jwt}`
3. 重新订阅当前所在频道（`subscribeChannel`）
4. 通知 React 层拉取最新数据（可能错过了离线期间的消息）

### 1.6 RESTClient

```typescript
class RESTClient {
  private auth: AuthManager;
  private baseUrl: string;

  async request<T>(method: string, path: string, body?: unknown): Promise<T>;
  // 1. auth.getValidToken() 获取 token
  // 2. fetch(baseUrl + path, { headers: { Authorization: `Bearer ${token}` } })
  // 3. 错误处理：401 → auth.logout(), 其他 → 抛出 ConstellError
}
```

**REST API 映射（读操作 + 认证 + 文件 + 通知）：**

| SDK 方法 | HTTP | 路径 |
|----------|------|------|
| `login` | POST | `/api/v1/auth/login` |
| `register` | POST | `/api/v1/auth/register` |
| `getUser` | GET | `/api/v1/users/{id}` |
| `listCommunities` | GET | `/api/v1/servers` (注意：后端用 server，SDK 映射为 community) |
| `getChannels` | GET | `/api/v1/servers/{id}/channels` |
| `getChannelHistory` | GET | `/api/v1/channels/{id}/messages` |
| `getDMHistory` | GET | `/api/v1/dm/history/{peerId}` |
| `getDMConversations` | GET | `/api/v1/dm/conversations` |
| `uploadFile` | POST | `/api/v1/files/upload` |
| `getFileURL` | GET | `/api/v1/files/{id}/url` |
| `search` | GET | `/api/v1/search?q=...` |
| `getUnreadCounts` | GET | `/api/v1/notify/unread` |
| `markDMRead` | POST | `/api/v1/notify/dm/{conv_id}/read` |
| `markChannelRead` | POST | `/api/v1/notify/channel/{ch_id}/read` |

**WS 消息（写操作——通过 WebSocket ClientMessage 发送，等待 ACK）：**

| SDK 方法 | ClientMessageType | 说明 |
|----------|-------------------|------|
| `sendDM` | `SEND_DM` | 发送私聊消息 |
| `sendChannelMessage` | `SEND_CHANNEL_MESSAGE` | 发送频道消息 |
| `subscribeChannel` | `SUBSCRIBE_CHANNEL` | 订阅频道事件 |
| `unsubscribeChannel` | `UNSUBSCRIBE_CHANNEL` | 取消订阅 |

> **术语映射：** 后端 API 使用 `server` / `server_id`，SDK-JS 在 TypeScript 类型层面统一使用 `community` / `communityId`。RESTClient 在发送请求时做字段名映射。

### 1.7 消息发送可靠性（乐观更新）

```
用户点击发送
  → SDK 生成 tempMsg { id: requestId, status: 'sending' }
  → emit('message_sending', tempMsg)  // React 立即显示灰色气泡
  → WS 发送 ClientMessage (SEND_DM 或 SEND_CHANNEL_MESSAGE)
  → 等待 ACK（5s 超时）
     → 收到 ACK → emit('message_sent', tempMsg)   // 气泡变正常
     → 超时/错误 → emit('message_failed', tempMsg) // 显示红色重发按钮
```

### 1.8 Proto 代码生成

```yaml
# sdk/sdk-js/buf.gen.yaml
version: v2
plugins:
  - remote: buf.build/bufbuild/es
    out: src/protobuf
    opt: target=ts
```

只生成 `gateway/v1/gateway.proto`（客户端协议），不生成后端服务层的 proto。

```bash
buf generate --template sdk/sdk-js/buf.gen.yaml
```

产出：`sdk/sdk-js/src/protobuf/gateway/v1/gateway_pb.ts`，包含 `ClientMessage`、`ServerEvent` 等完整 TS 类型。

---

## 2. React Web 应用设计

### 2.1 目录结构

```
clients/web/
├── src/
│   ├── main.tsx                    # 入口
│   ├── App.tsx                     # 路由配置
│   ├── pages/
│   │   ├── LoginPage.tsx
│   │   ├── RegisterPage.tsx
│   │   └── MainPage.tsx            # MainLayout 容器
│   ├── components/
│   │   ├── ui/                     # shadcn/ui 组件（Button, Input, Avatar, etc.）
│   │   ├── layout/
│   │   │   ├── MainLayout.tsx      # 三栏布局容器
│   │   │   ├── CommunityRail.tsx   # 左栏 72px Community 图标轨道
│   │   │   ├── ChannelList.tsx     # 中栏 Channel/DM 列表
│   │   │   └── MemberList.tsx      # 右栏成员列表（仅 Community 视图）
│   │   ├── chat/
│   │   │   ├── MessageList.tsx     # 消息列表（虚拟滚动）
│   │   │   ├── MessageBubble.tsx   # 单条消息气泡
│   │   │   ├── ChatInput.tsx       # 消息输入框 + 文件上传
│   │   │   └── ChatHeader.tsx      # 频道/DM 顶部信息
│   │   ├── dm/
│   │   │   ├── DMList.tsx          # DM 对话列表
│   │   │   └── DMChat.tsx          # DM 聊天视图
│   │   ├── search/
│   │   │   └── SearchDialog.tsx    # 搜索弹窗 (Cmd+K)
│   │   └── auth/
│   │       ├── LoginForm.tsx
│   │       └── RegisterForm.tsx
│   ├── stores/
│   │   ├── authStore.ts
│   │   ├── communitiesStore.ts
│   │   ├── messagesStore.ts
│   │   ├── unreadStore.ts
│   │   └── uiStore.ts
│   ├── hooks/
│   │   ├── useConstellClient.ts    # React Context 获取 SDK 实例
│   │   ├── useClientEvents.ts      # SDK 事件 → Store 桥接
│   │   ├── useAuth.ts              # 认证状态 hook
│   │   ├── useChat.ts              # 聊天操作 hook
│   │   ├── useUnread.ts            # 未读状态 hook
│   │   └── useOnlineStatus.ts      # 在线状态 hook
│   ├── lib/
│   │   ├── client.ts               # ConstellClient 单例创建
│   │   └── utils.ts                # cn() 等 shadcn 工具函数
│   └── styles/
│       └── globals.css             # Tailwind + 自定义 CSS 变量（深色主题）
├── public/
├── index.html
├── package.json
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
├── components.json                 # shadcn/ui 配置
├── nginx.conf                      # 生产 Nginx 配置
├── Dockerfile                      # 多阶段构建
└── .gitignore
```

### 2.2 路由

使用 React Router v6：

| 路径 | 组件 | 需认证 | 说明 |
|------|------|--------|------|
| `/login` | LoginPage | 否 | 登录表单 |
| `/register` | RegisterPage | 否 | 注册表单 |
| `/` | MainLayout | 是 | 三栏布局容器 |
| `/@me` | DMList | 是 | DM 对话列表 |
| `/@me/:peerId` | DMChat | 是 | 与某人的 DM 对话 |
| `/:communityId` | ChannelView (首个频道) | 是 | Community 的默认频道 |
| `/:communityId/:channelId` | ChannelView | 是 | 频道消息视图 |

**认证守卫：** 未认证用户访问 `/` 开头路由时重定向到 `/login`。

### 2.3 三栏布局

```
┌──────────┬────────────┬─────────────────────────────────────┬────────────┐
│ 72px     │ 240px      │ flex                                │ 240px      │
│          │            │                                     │ (可选)      │
│ Community│ Channel    │ Chat Area                           │ Member     │
│ Rail     │ / DM List  │                                     │ List       │
│          │            │ ┌───────────────────────────────┐   │            │
│ 💬 DM    │ # general  │ │ ChatHeader (频道名/搜索/成员)   │   │ Online: 3  │
│ ────     │ # random   │ ├───────────────────────────────┤   │ ● Alice    │
│ [C1]     │ # dev      │ │                               │   │ ● Bob      │
│ [C2]     │            │ │ MessageList                   │   │ ● You      │
│ [C3]     │ DMs ─────  │ │ (虚拟滚动消息列表)              │   │            │
│          │ ● Alice    │ │                               │   │ Offline: 2 │
│ [+ 创建] │ ○ Bob      │ │                               │   │ ○ Charlie  │
│          │ ○ Charlie  │ ├───────────────────────────────┤   │            │
│ [我的    │            │ │ ChatInput (输入框/文件上传/表情) │   │            │
│  头像]   │ 🔍 搜索     │ └───────────────────────────────┘   │            │
└──────────┴────────────┴─────────────────────────────────────┴────────────┘
```

- **Member List (240px)**：仅在 Community 频道视图显示，DM 视图隐藏
- **DM 模式**：中栏显示 DM 对话列表，点击进入聊天
- **Community 模式**：中栏显示 Text Channels 分组列表

### 2.4 Zustand Store 设计

#### authStore

```typescript
interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<void>;
  logout: () => void;
  initAuth: () => void; // 从 localStorage 恢复 session
}
```

#### communitiesStore

```typescript
interface CommunitiesState {
  communities: Map<string, Community>;
  channels: Map<string, Channel[]>;       // communityId → channels
  currentCommunityId: string | null;
  currentChannelId: string | null;
  fetchCommunities: () => Promise<void>;
  selectCommunity: (id: string) => void;
  selectChannel: (id: string) => void;
  createCommunity: (name: string, description?: string) => Promise<void>;
}
```

#### messagesStore

```typescript
interface MessagesState {
  channelMessages: Map<string, ChannelMessage[]>;  // channelId → messages
  dmMessages: Map<string, DMMessage[]>;             // peerId → messages
  loading: boolean;
  fetchChannelHistory: (channelId: string) => Promise<void>;
  fetchDMHistory: (peerId: string) => Promise<void>;
  sendMessage: (target: { type: 'channel' | 'dm'; id: string }, content: string, fileIds?: string[]) => Promise<void>;
  appendMessage: (type: 'channel' | 'dm', targetId: string, message: Message) => void;
}
```

#### unreadStore

```typescript
interface UnreadState {
  dmUnreads: Map<string, number>;       // peerId → 未读数
  channelUnreads: Map<string, number>;  // channelId → 未读数
  fetchUnreads: () => Promise<void>;
  markDMRead: (peerId: string) => Promise<void>;
  markChannelRead: (channelId: string) => Promise<void>;
  incrementUnread: (type: 'dm' | 'channel', id: string) => void;
}
```

#### uiStore

```typescript
interface UIState {
  view: 'community' | 'dm';
  showMemberList: boolean;
  showSearch: boolean;
  onlineUsers: Set<string>;
  wsStatus: 'connected' | 'connecting' | 'disconnected';
  setView: (view: 'community' | 'dm') => void;
  toggleMemberList: () => void;
  toggleSearch: () => void;
  setOnline: (userId: string) => void;
  setOffline: (userId: string) => void;
  setWsStatus: (status: 'connected' | 'connecting' | 'disconnected') => void;
}
```

### 2.5 实时数据流

SDK 事件通过 `useClientEvents` hook 桥接到 Zustand Store：

```
SDK Event                   →  Store Action                  →  UI Effect
─────────────────────────────────────────────────────────────────────────────
dm_received                 →  messagesStore.appendMessage   →  消息列表新增气泡
                               unreadStore.incrementUnread   →  DM 列表红点 +1
channel_message             →  messagesStore.appendMessage   →  频道消息列表新增
                               unreadStore.incrementUnread   →  频道红点 +1
user_online                 →  uiStore.setOnline             →  成员列表绿点亮起
user_offline                →  uiStore.setOffline            →  成员列表绿点熄灭
notification                →  unreadStore.updateFromNotif   →  更新未读角标
disconnected                →  uiStore.setWsStatus           →  顶部连接状态提示
reconnected                 →  uiStore.setWsStatus           →  恢复正常
message_sending             →  messagesStore.appendMessage   →  灰色待确认气泡
message_sent / _failed      →  messagesStore.updateStatus    →  气泡状态更新
```

**`useClientEvents` hook：** 在 MainLayout 顶层调用一次，全局生效。使用 `useEffect` 注册 SDK 事件监听，cleanup 时 `removeAllListeners`。

### 2.6 消息列表虚拟滚动

频道消息可能有大量历史消息，使用虚拟滚动优化：

- 使用 `@tanstack/react-virtual` 实现虚拟化
- 只渲染可视区域 ± buffer 的消息 DOM
- 向上滚动到顶部时自动加载更多历史（`fetchChannelHistory` 追加）
- 收到新消息时自动滚动到底部（如果在底部）

### 2.7 搜索 UI

- `Cmd+K` / `Ctrl+K` 全局快捷键打开搜索弹窗 (shadcn Command 组件)
- 实时搜索：输入 300ms debounce 后调 `client.search(query)`
- 结果分类显示：用户 / 频道消息 / DM 消息
- 点击结果跳转到对应频道/DM

### 2.8 文件上传 UI

- ChatInput 区域的 `+` 按钮触发文件选择
- 拖拽文件到 ChatInput 也可触发上传
- 上传流程：
  1. 选择文件 → `client.uploadFile(file)` → 获得 `file_id`
  2. 在输入框中显示附件预览（图片缩略图 / 文件名）
  3. 发送消息时将 `file_ids` 一并发送
- 上传进度条（使用 XMLHttpRequest 的 progress 事件）

### 2.9 未读标记

- Community 图标上的红点数字：该 Community 下所有频道的未读总数
- Channel 名称上的红点数字
- DM 列表中用户名旁的红点数字
- `unreadStore` 作为唯一数据源，由 SDK `notification` 事件和 `getUnreadCounts()` 初始化

### 2.10 深色主题

使用 Tailwind CSS 变量 + shadcn/ui dark 模式。配色基于 Catppuccin Mocha：

```css
/* globals.css */
:root {
  --background: 30 10% 11%;      /* #1e1e2e */
  --foreground: 267 84% 94%;     /* #cdd6f4 */
  --card: 30 10% 14%;            /* #181825 */
  --primary: 263 70% 58%;        /* #7c3aed */
  --muted: 267 11% 38%;          /* #585b70 */
  --accent: 267 11% 25%;         /* #313244 */
  --destructive: 0 72% 61%;      /* #f38ba8 */
  /* ... 其他 shadcn CSS 变量 */
}
```

---

## 3. 部署方案

### 3.1 开发环境

```bash
# 终端 1: 后端服务
make docker-up  # PG + Redis + NATS + MinIO + 所有后端服务

# 终端 2: SDK-JS 开发
cd sdk/sdk-js
npm run dev      # tsc --watch
npm run test     # Vitest

# 终端 3: Web 应用开发
cd clients/web
npm run dev      # Vite dev server → http://localhost:5173
```

Vite 代理配置：

```typescript
// clients/web/vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8080',     // REST → API Gateway
      '/ws': {
        target: 'ws://localhost:8081',      // WS → WS Gateway
        ws: true,
      },
    },
  },
});
```

### 3.2 生产部署

**Dockerfile (多阶段构建)：**

```dockerfile
# clients/web/Dockerfile
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

**Nginx 配置：**

```nginx
server {
    listen 80;

    # 前端静态文件 (SPA fallback)
    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }

    # REST API 反代
    location /api/ {
        proxy_pass http://api-gateway:8080;
    }

    # WebSocket 反代（轮询多台 WS Gateway）
    # MVP 用 upstream 轮询，后续可改为 sticky session
    upstream ws_gateways {
        server ws-gateway-1:8081;
        server ws-gateway-2:8081;
    }
    location /ws {
        proxy_pass http://ws_gateways;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

**Docker Compose 新增：**

```yaml
web:
  build:
    context: ../../clients/web
    dockerfile: Dockerfile
  ports:
    - "3000:80"
  depends_on:
    - api-gateway
    - ws-gateway-1
```

### 3.3 SDK-JS 发布

```json
// sdk/sdk-js/package.json
{
  "name": "@constell/sdk-js",
  "type": "module",
  "main": "./dist/index.cjs",
  "module": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "exports": {
    ".": {
      "import": "./dist/index.js",
      "require": "./dist/index.cjs"
    }
  },
  "dependencies": {
    "@bufbuild/protobuf": "^2.0.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0",
    "vitest": "^3.0.0"
  }
}
```

ESM + CJS 双格式输出。零 DOM 依赖，可在 Node.js 环境使用。

---

## 4. 测试策略

### 4.1 SDK-JS 测试 (Vitest)

| 测试类型 | 范围 | 工具 |
|----------|------|------|
| 单元测试 | AuthManager, RESTClient, codec | Vitest + MSW (mock HTTP) |
| 集成测试 | ConstellClient 全流程 | Vitest + mock WebSocket |
| Proto 测试 | 编解码正确性 | 生成数据 vs 手工构造 |

### 4.2 React 应用测试

| 测试类型 | 范围 | 工具 |
|----------|------|------|
| 组件测试 | 关键组件渲染 | Vitest + Testing Library |
| Store 测试 | Zustand store actions | Vitest |
| E2E 测试 | 关键用户流程 | Playwright (后续) |

---

## 5. 关键依赖版本

| 依赖 | 版本 | 用途 |
|------|------|------|
| React | 18/19 | UI 框架 |
| TypeScript | ^5.0 | 类型安全 |
| Vite | ^6.0 | 构建工具 |
| Zustand | ^5.0 | 状态管理 |
| React Router | ^7.0 | 路由 |
| Tailwind CSS | ^4.0 | 样式 |
| shadcn/ui | latest | UI 组件 |
| @bufbuild/protobuf | ^2.0 | Protobuf JS 库 |
| @tanstack/react-virtual | ^3.0 | 虚拟滚动 |
| Vitest | ^3.0 | 测试 |
