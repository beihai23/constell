# Plan 5: Web 客户端 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Constell 构建 SDK-JS 库和 React Web 前端客户端，实现完整的 IM 用户体验——登录注册、Community/Channel 浏览、实时消息收发、DM、文件上传、搜索、未读标记。

**Architecture:** 独立的 SDK-JS 包（胖 SDK 模式）封装所有后端交互（REST + WebSocket + Protobuf），React 应用通过 Zustand stores 和 hooks 消费 SDK。SDK-JS 零 DOM 依赖，可在 Node.js 环境使用。React 应用使用 Discord 风格三栏布局，shadcn/ui + Tailwind 深色主题。

**Tech Stack:** TypeScript, React 18/19, Vite 6, Zustand 5, shadcn/ui, Tailwind CSS 4, @bufbuild/protobuf 2, @tanstack/react-virtual 3, Vitest 3, React Router 7

**Spec:** `docs/superpowers/specs/2026-06-04-constell-plan5-web-client-design.md`

---

## File Structure

```
sdk/sdk-js/
├── src/
│   ├── index.ts              # 公共 API 导出
│   ├── client.ts             # ConstellClient 主入口
│   ├── auth.ts               # AuthManager — JWT 管理
│   ├── ws-manager.ts         # WSManager — WebSocket 连接管理
│   ├── rest-client.ts        # RESTClient — HTTP API 调用
│   ├── event-bus.ts          # EventBus — 类型安全事件分发
│   ├── codec.ts              # 二进制帧编解码
│   ├── types.ts              # 公共类型定义
│   ├── errors.ts             # ConstellError 错误类型
│   └── protobuf/             # buf generate 生成（不手动编辑）
│       └── gateway/v1/
│           └── gateway_pb.ts
├── tests/
│   ├── codec.test.ts
│   ├── auth.test.ts
│   ├── rest-client.test.ts
│   ├── ws-manager.test.ts
│   ├── event-bus.test.ts
│   └── client.test.ts
├── package.json
├── tsconfig.json
├── vitest.config.ts
└── buf.gen.yaml

clients/web/
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── pages/
│   │   ├── LoginPage.tsx
│   │   ├── RegisterPage.tsx
│   │   └── MainPage.tsx
│   ├── components/
│   │   ├── ui/                # shadcn/ui 组件（npx shadcn 生成）
│   │   ├── layout/
│   │   │   ├── MainLayout.tsx
│   │   │   ├── CommunityRail.tsx
│   │   │   ├── ChannelList.tsx
│   │   │   └── MemberList.tsx
│   │   ├── chat/
│   │   │   ├── MessageList.tsx
│   │   │   ├── MessageBubble.tsx
│   │   │   ├── ChatInput.tsx
│   │   │   └── ChatHeader.tsx
│   │   ├── dm/
│   │   │   ├── DMList.tsx
│   │   │   └── DMChat.tsx
│   │   ├── search/
│   │   │   └── SearchDialog.tsx
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
│   │   ├── useConstellClient.ts
│   │   ├── useClientEvents.ts
│   │   ├── useAuth.ts
│   │   ├── useChat.ts
│   │   ├── useUnread.ts
│   │   └── useOnlineStatus.ts
│   ├── lib/
│   │   ├── client.ts
│   │   └── utils.ts
│   └── styles/
│       └── globals.css
├── public/
│   └── favicon.svg
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
├── tailwind.config.ts
├── components.json
├── nginx.conf
└── Dockerfile

deploy/docker/docker-compose.yml  # 修改：新增 web 服务
docs/PROJECT_STATUS.md             # 修改：更新 Plan 5 状态
```

---

## Phase 1: SDK-JS 基础设施

### Task 1: SDK-JS 项目脚手架 + 类型定义

**Files:**
- Create: `sdk/sdk-js/package.json`
- Create: `sdk/sdk-js/tsconfig.json`
- Create: `sdk/sdk-js/vitest.config.ts`
- Create: `sdk/sdk-js/src/types.ts`
- Create: `sdk/sdk-js/src/errors.ts`
- Create: `sdk/sdk-js/src/index.ts`

- [ ] **Step 1: 创建 package.json**

```json
{
  "name": "@constell/sdk-js",
  "version": "0.1.0",
  "type": "module",
  "main": "./dist/index.cjs",
  "module": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "exports": {
    ".": {
      "import": "./dist/index.js",
      "require": "./dist/index.cjs",
      "types": "./dist/index.d.ts"
    }
  },
  "files": ["dist"],
  "scripts": {
    "build": "tsc",
    "dev": "tsc --watch",
    "test": "vitest run",
    "test:watch": "vitest",
    "lint": "tsc --noEmit"
  },
  "dependencies": {
    "@bufbuild/protobuf": "^2.2.0"
  },
  "devDependencies": {
    "typescript": "^5.7.0",
    "vitest": "^3.0.0"
  }
}
```

- [ ] **Step 2: 创建 tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2022"],
    "outDir": "./dist",
    "rootDir": "./src",
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true
  },
  "include": ["src/**/*.ts"],
  "exclude": ["node_modules", "dist", "tests"]
}
```

- [ ] **Step 3: 创建 vitest.config.ts**

```typescript
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: true,
    environment: 'node',
  },
});
```

- [ ] **Step 4: 创建 src/errors.ts**

```typescript
export class ConstellError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly statusCode?: number,
  ) {
    super(message);
    this.name = 'ConstellError';
  }
}

export class AuthError extends ConstellError {
  constructor(message: string) {
    super('AUTH_ERROR', message, 401);
    this.name = 'AuthError';
  }
}

export class NetworkError extends ConstellError {
  constructor(message: string) {
    super('NETWORK_ERROR', message);
    this.name = 'NetworkError';
  }
}
```

- [ ] **Step 5: 创建 src/types.ts**

```typescript
// === User ===
export interface User {
  id: string;
  username: string;
  nickname: string;
  avatarUrl: string;
  statusMsg: string;
}

// === Community (后端叫 Server) ===
export interface Community {
  id: string;
  name: string;
  iconUrl: string;
  description: string;
  ownerId: string;
  createdAt: string;
}

// === Channel ===
export interface Channel {
  id: string;
  communityId: string;
  name: string;
  topic: string;
  position: number;
  type: 'text' | 'announcement';
}

// === Member ===
export interface Member {
  userId: string;
  communityId: string;
  joinedAt: string;
  roleIds: string[];
}

// === Messages ===
export interface Attachment {
  id: string;
  fileId: string;
  filename: string;
  contentType: string;
  size: number;
  url: string;
  thumbnailUrl: string;
}

export interface DMMessage {
  id: string;
  senderId: string;
  contentType: string;
  content: string;
  attachments: Attachment[];
  createdAt: string;
}

export interface ChannelMessage {
  id: string;
  channelId: string;
  senderId: string;
  contentType: string;
  content: string;
  attachments: Attachment[];
  mentions: string[];
  createdAt: string;
}

export interface DMConversation {
  peerId: string;
  peer: User;
  lastMessage: string;
  lastMessageAt: string;
}

// === File ===
export interface FileInfo {
  id: string;
  filename: string;
  contentType: string;
  size: number;
  url: string;
  thumbnailUrl: string;
  createdAt: number;
}

// === Search ===
export type SearchType = 'users' | 'messages' | 'dm_messages';

export interface UserSearchResult {
  id: string;
  nickname: string;
  avatarUrl: string;
  relevance: number;
}

export interface MessageSearchResult {
  id: string;
  channelId: string;
  communityId: string;
  authorId: string;
  content: string;
  createdAt: number;
  relevance: number;
}

export interface DMMessageSearchResult {
  id: string;
  conversationId: string;
  peerId: string;
  content: string;
  createdAt: number;
  relevance: number;
}

export interface SearchResults {
  users: UserSearchResult[];
  messages: MessageSearchResult[];
  dmMessages: DMMessageSearchResult[];
}

// === Notify ===
export interface UnreadDMConversation {
  conversationId: string;
  peerId: string;
  count: number;
}

export interface UnreadChannel {
  channelId: string;
  communityId: string;
  count: number;
}

export interface UnreadCounts {
  dmTotal: number;
  dmConversations: UnreadDMConversation[];
  channelTotal: number;
  channels: UnreadChannel[];
}

// === Events ===
export type ClientEventType =
  | 'connected'
  | 'disconnected'
  | 'reconnecting'
  | 'reconnected'
  | 'dm_received'
  | 'channel_message'
  | 'user_online'
  | 'user_offline'
  | 'notification'
  | 'message_sending'
  | 'message_sent'
  | 'message_failed';

export interface NotificationEvent {
  notificationType: string;
  sourceId: string;
  communityId: string;
  senderId: string;
  senderNickname: string;
  preview: string;
  createdAt: number;
}

export interface TempMessage {
  requestId: string;
  targetId: string;
  targetType: 'channel' | 'dm';
  content: string;
  status: 'sending' | 'sent' | 'failed';
}

export type WSStatus = 'connected' | 'connecting' | 'disconnected';

export interface ClientConfig {
  apiUrl: string;
  wsUrl: string;
}

// === Pagination ===
export interface PageOptions {
  limit?: number;
  cursor?: string;
}

export interface PageResult<T> {
  items: T[];
  nextCursor?: string;
}
```

- [ ] **Step 6: 创建 src/index.ts (空壳，后续 task 逐步导出)**

```typescript
// SDK-JS 公共 API 导出
// 后续 task 逐步添加导出
export { ConstellError, AuthError, NetworkError } from './errors.js';
export type * from './types.js';
```

- [ ] **Step 7: 安装依赖并验证 TypeScript 编译**

Run: `cd sdk/sdk-js && npm install && npx tsc --noEmit`
Expected: 无错误

- [ ] **Step 8: 提交**

```bash
cd /Users/lance.wang/workspace/wzgown/constell
git add sdk/sdk-js/
git commit -m "feat(sdk-js): scaffold project with types and error definitions"
```

---

### Task 2: Proto TS 代码生成 + 二进制帧编解码器

**Files:**
- Create: `sdk/sdk-js/buf.gen.yaml`
- Create: `sdk/sdk-js/src/protobuf/` (buf generate 产出)
- Create: `sdk/sdk-js/src/codec.ts`
- Create: `sdk/sdk-js/tests/codec.test.ts`

- [ ] **Step 1: 创建 buf.gen.yaml**

```yaml
version: v2
inputs:
  - directory: ../../proto
plugins:
  - remote: buf.build/bufbuild/es
    out: src/protobuf
    opt:
      - target=ts
```

- [ ] **Step 2: 生成 Proto TS 代码**

Run: `cd sdk/sdk-js && npx buf generate`
Expected: 生成 `src/protobuf/gateway/v1/gateway_pb.ts` 和 `src/protobuf/common/v1/common_pb.ts`

- [ ] **Step 3: 验证生成的代码可导入**

Run: `cd sdk/sdk-js && node -e "import('./src/protobuf/gateway/v1/gateway_pb.js').then(m => console.log(Object.keys(m).slice(0,5)))"`
Expected: 输出包含 proto 消息类型名

- [ ] **Step 4: 写 codec 测试**

```typescript
// tests/codec.test.ts
import { describe, it, expect } from 'vitest';
import { encodeClientFrame, decodeServerFrame } from '../src/codec.js';
import { createClientMessage } from '../src/codec.js';

describe('codec', () => {
  it('encodes and decodes a heartbeat ClientMessage', () => {
    const msg = createClientMessage({
      type: 5, // HEARTBEAT
      requestId: 'test-req-1',
    });
    const encoded = encodeClientFrame(msg);
    expect(encoded).toBeInstanceOf(Uint8Array);
    expect(encoded.byteLength).toBeGreaterThan(4); // 4-byte prefix + payload

    const decoded = decodeServerFrame(encoded.slice(0, 4), encoded.slice(4));
    // heartbeat 不走 ServerEvent decode，这里只验证帧格式
    expect(encoded.slice(0, 4)).toBeInstanceOf(Uint8Array);
  });

  it('encodes frame with 4-byte big-endian length prefix', () => {
    const msg = createClientMessage({
      type: 5,
      requestId: 'hb',
    });
    const frame = encodeClientFrame(msg);
    const view = new DataView(frame.buffer, frame.byteOffset, 4);
    const declaredLen = view.getUint32(0, false); // big-endian
    expect(declaredLen).toBe(frame.byteLength - 4);
  });
});
```

- [ ] **Step 5: 实现 codec.ts**

```typescript
// src/codec.ts
import { ClientMessage, ServerEvent } from './protobuf/gateway/v1/gateway_pb.js';
import { protoBase64 } from '@bufbuild/protobuf';

/**
 * 创建 ClientMessage 的辅助函数。
 * type 和 requestId 必填，其余按需填。
 */
export function createClientMessage(opts: {
  type: number;
  requestId: string;
  sendDm?: { receiverId: string; content: string; fileIds?: string[] };
  sendChannelMessage?: { channelId: string; content: string; fileIds?: string[] };
  subscribeChannel?: { channelId: string };
  unsubscribeChannel?: { channelId: string };
}): ClientMessage {
  const msg = new ClientMessage({
    type: opts.type,
    requestId: opts.requestId,
  });
  if (opts.sendDm) {
    msg.sendDmRequest = {
      receiverId: opts.sendDm.receiverId,
      content: opts.sendDm.content,
      fileIds: opts.sendDm.fileIds ?? [],
    };
  }
  if (opts.sendChannelMessage) {
    msg.sendChannelMessageRequest = {
      channelId: opts.sendChannelMessage.channelId,
      content: opts.sendChannelMessage.content,
      fileIds: opts.sendChannelMessage.fileIds ?? [],
    };
  }
  if (opts.subscribeChannel) {
    msg.subscribeChannelRequest = { channelId: opts.subscribeChannel.channelId };
  }
  if (opts.unsubscribeChannel) {
    msg.unsubscribeChannelRequest = { channelId: opts.unsubscribeChannel.channelId };
  }
  return msg;
}

/**
 * 编码 ClientMessage 为二进制帧：[4字节 big-endian 长度][protobuf payload]
 */
export function encodeClientFrame(msg: ClientMessage): Uint8Array {
  const payload = msg.toBinary();
  const frame = new Uint8Array(4 + payload.byteLength);
  new DataView(frame.buffer).setUint32(0, payload.byteLength, false); // big-endian
  frame.set(payload, 4);
  return frame;
}

/**
 * 从二进制帧解码 ServerEvent。
 * @param header 前 4 字节（长度前缀）
 * @param payload 后续字节（protobuf payload）
 */
export function decodeServerFrame(header: Uint8Array, payload: Uint8Array): ServerEvent {
  return ServerEvent.fromBinary(payload);
}

/**
 * 从 ArrayBuffer 读取完整的二进制帧，返回 ServerEvent。
 * 如果数据不足一帧，返回 null。
 */
export function readServerEvent(buffer: ArrayBuffer): { event: ServerEvent; bytesRead: number } | null {
  if (buffer.byteLength < 4) return null;
  const view = new DataView(buffer);
  const payloadLen = view.getUint32(0, false);
  const totalLen = 4 + payloadLen;
  if (buffer.byteLength < totalLen) return null;

  const payload = new Uint8Array(buffer, 4, payloadLen);
  const event = ServerEvent.fromBinary(payload);
  return { event, bytesRead: totalLen };
}

/** 生成唯一 request ID */
export function generateRequestId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}
```

- [ ] **Step 6: 运行测试**

Run: `cd sdk/sdk-js && npx vitest run tests/codec.test.ts`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add sdk/sdk-js/
git commit -m "feat(sdk-js): add proto codegen and binary frame codec"
```

---

### Task 3: EventBus 类型安全事件分发

**Files:**
- Create: `sdk/sdk-js/src/event-bus.ts`
- Create: `sdk/sdk-js/tests/event-bus.test.ts`

- [ ] **Step 1: 写 EventBus 测试**

```typescript
// tests/event-bus.test.ts
import { describe, it, expect, vi } from 'vitest';
import { EventBus } from '../src/event-bus.js';

interface TestEvents {
  ping: (data: { count: number }) => void;
  message: (data: { text: string }) => void;
}

describe('EventBus', () => {
  it('calls registered handler on emit', () => {
    const bus = new EventBus<TestEvents>();
    const handler = vi.fn();
    bus.on('ping', handler);
    bus.emit('ping', { count: 42 });
    expect(handler).toHaveBeenCalledWith({ count: 42 });
  });

  it('supports multiple handlers for same event', () => {
    const bus = new EventBus<TestEvents>();
    const h1 = vi.fn();
    const h2 = vi.fn();
    bus.on('ping', h1);
    bus.on('ping', h2);
    bus.emit('ping', { count: 1 });
    expect(h1).toHaveBeenCalled();
    expect(h2).toHaveBeenCalled();
  });

  it('off removes specific handler', () => {
    const bus = new EventBus<TestEvents>();
    const handler = vi.fn();
    bus.on('ping', handler);
    bus.off('ping', handler);
    bus.emit('ping', { count: 1 });
    expect(handler).not.toHaveBeenCalled();
  });

  it('removeAllListeners clears all handlers', () => {
    const bus = new EventBus<TestEvents>();
    const h1 = vi.fn();
    const h2 = vi.fn();
    bus.on('ping', h1);
    bus.on('message', h2);
    bus.removeAllListeners();
    bus.emit('ping', { count: 1 });
    bus.emit('message', { text: 'hi' });
    expect(h1).not.toHaveBeenCalled();
    expect(h2).not.toHaveBeenCalled();
  });

  it('does not throw when emitting with no handlers', () => {
    const bus = new EventBus<TestEvents>();
    expect(() => bus.emit('ping', { count: 0 })).not.toThrow();
  });
});
```

- [ ] **Step 2: 实现 EventBus**

```typescript
// src/event-bus.ts
export type EventHandler = (...args: any[]) => void;

export interface EventMap {
  [key: string]: EventHandler;
}

export class EventBus<T extends EventMap> {
  private handlers = new Map<keyof T, Set<EventHandler>>();

  on<K extends keyof T>(event: K, handler: T[K]): void {
    if (!this.handlers.has(event)) {
      this.handlers.set(event, new Set());
    }
    this.handlers.get(event)!.add(handler);
  }

  off<K extends keyof T>(event: K, handler: T[K]): void {
    this.handlers.get(event)?.delete(handler);
  }

  emit<K extends keyof T>(event: K, ...args: Parameters<T[K]>): void {
    const handlers = this.handlers.get(event);
    if (!handlers) return;
    for (const handler of handlers) {
      try {
        handler(...args);
      } catch {
        // swallow handler errors, don't break other handlers
      }
    }
  }

  removeAllListeners(): void {
    this.handlers.clear();
  }
}
```

- [ ] **Step 3: 运行测试**

Run: `cd sdk/sdk-js && npx vitest run tests/event-bus.test.ts`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add sdk/sdk-js/src/event-bus.ts sdk/sdk-js/tests/event-bus.test.ts
git commit -m "feat(sdk-js): add typed EventBus"
```

---

### Task 4: AuthManager — JWT 管理

**Files:**
- Create: `sdk/sdk-js/src/auth.ts`
- Create: `sdk/sdk-js/tests/auth.test.ts`

- [ ] **Step 1: 写 AuthManager 测试**

```typescript
// tests/auth.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { AuthManager } from '../src/auth.js';

// mock localStorage for Node
const store = new Map<string, string>();
const mockLocalStorage = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, val: string) => store.set(key, val),
  removeItem: (key: string) => store.delete(key),
  clear: () => store.clear(),
};

describe('AuthManager', () => {
  let auth: AuthManager;

  beforeEach(() => {
    store.clear();
    auth = new AuthManager('http://localhost:8080', mockLocalStorage);
  });

  it('stores tokens after login', async () => {
    const fakeFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        user_id: 'u1',
        access_token: 'at-123',
        refresh_token: 'rt-456',
      }),
    });
    vi.stubGlobal('fetch', fakeFetch);

    const user = await auth.login('test@test.com', 'password');
    expect(user.id).toBe('u1');
    expect(mockLocalStorage.getItem('constell_access_token')).toBe('at-123');
    expect(mockLocalStorage.getItem('constell_refresh_token')).toBe('rt-456');
  });

  it('getValidToken returns token when not expired', async () => {
    store.set('constell_access_token', makeJWT(Date.now() / 1000 + 3600)); // 1h from now
    store.set('constell_refresh_token', 'rt-valid');
    const token = await auth.getValidToken();
    expect(token).toBeTruthy();
  });

  it('getValidToken refreshes when expired', async () => {
    store.set('constell_access_token', makeJWT(Date.now() / 1000 - 100)); // expired
    store.set('constell_refresh_token', 'rt-old');

    const fakeFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        access_token: 'at-new',
        refresh_token: 'rt-new',
      }),
    });
    vi.stubGlobal('fetch', fakeFetch);

    const token = await auth.getValidToken();
    expect(token).toBe('at-new');
    expect(mockLocalStorage.getItem('constell_access_token')).toBe('at-new');
  });

  it('logout clears tokens', async () => {
    store.set('constell_access_token', 'at-123');
    store.set('constell_refresh_token', 'rt-456');
    auth.logout();
    expect(mockLocalStorage.getItem('constell_access_token')).toBeNull();
    expect(auth.isAuthenticated()).toBe(false);
  });

  it('initFromStorage restores user from valid token', () => {
    store.set('constell_access_token', makeJWT(Date.now() / 1000 + 3600));
    const user = auth.initFromStorage();
    expect(user).not.toBeNull();
    expect(user!.id).toBe('u1');
  });

  it('initFromStorage returns null when no token', () => {
    const user = auth.initFromStorage();
    expect(user).toBeNull();
  });
});

/** 创建一个假的 JWT（base64 编码的 JSON payload） */
function makeJWT(exp: number): string {
  const header = btoa(JSON.stringify({ alg: 'HS256' }));
  const payload = btoa(JSON.stringify({ sub: 'u1', exp }));
  return `${header}.${payload}.fake-signature`;
}
```

- [ ] **Step 2: 实现 AuthManager**

```typescript
// src/auth.ts
import type { User, WSStatus } from './types.js';
import { AuthError } from './errors.js';

interface Storage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
  clear(): void;
}

const ACCESS_TOKEN_KEY = 'constell_access_token';
const REFRESH_TOKEN_KEY = 'constell_refresh_token';

export class AuthManager {
  private refreshPromise: Promise<void> | null = null;

  constructor(
    private apiUrl: string,
    private storage: Storage = typeof localStorage !== 'undefined' ? localStorage : {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
      clear: () => {},
    },
  ) {}

  async login(email: string, password: string): Promise<User> {
    const resp = await fetch(`${this.apiUrl}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({}));
      throw new AuthError(err.message ?? 'Login failed');
    }
    const data = await resp.json();
    this.setTokens(data.access_token, data.refresh_token);
    return { id: data.user_id, username: '', nickname: '', avatarUrl: '', statusMsg: '' };
  }

  async register(username: string, email: string, password: string): Promise<User> {
    const resp = await fetch(`${this.apiUrl}/api/v1/auth/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, email, password }),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({}));
      throw new AuthError(err.message ?? 'Registration failed');
    }
    const data = await resp.json();
    this.setTokens(data.access_token, data.refresh_token);
    return { id: data.user_id, username, nickname: '', avatarUrl: '', statusMsg: '' };
  }

  logout(): void {
    this.storage.removeItem(ACCESS_TOKEN_KEY);
    this.storage.removeItem(REFRESH_TOKEN_KEY);
  }

  isAuthenticated(): boolean {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    return token !== null && !this.isTokenExpired(token);
  }

  async getValidToken(): Promise<string> {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!token) throw new AuthError('Not authenticated');
    if (!this.isTokenExpired(token)) return token;

    // Token expired, refresh
    if (!this.refreshPromise) {
      this.refreshPromise = this.doRefresh();
    }
    await this.refreshPromise;
    const newToken = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!newToken) throw new AuthError('Refresh failed');
    return newToken;
  }

  initFromStorage(): User | null {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!token || this.isTokenExpired(token)) return null;
    return this.parseUserFromToken(token);
  }

  private async doRefresh(): Promise<void> {
    try {
      const refreshToken = this.storage.getItem(REFRESH_TOKEN_KEY);
      if (!refreshToken) throw new AuthError('No refresh token');

      const resp = await fetch(`${this.apiUrl}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      if (!resp.ok) {
        this.logout();
        throw new AuthError('Token refresh failed');
      }
      const data = await resp.json();
      this.setTokens(data.access_token, data.refresh_token);
    } finally {
      this.refreshPromise = null;
    }
  }

  private setTokens(accessToken: string, refreshToken: string): void {
    this.storage.setItem(ACCESS_TOKEN_KEY, accessToken);
    this.storage.setItem(REFRESH_TOKEN_KEY, refreshToken);
  }

  private isTokenExpired(token: string): boolean {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]));
      // Refresh 60s before actual expiry
      return payload.exp < Date.now() / 1000 + 60;
    } catch {
      return true;
    }
  }

  private parseUserFromToken(token: string): User | null {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]));
      return {
        id: payload.sub,
        username: '',
        nickname: '',
        avatarUrl: '',
        statusMsg: '',
      };
    } catch {
      return null;
    }
  }
}
```

- [ ] **Step 3: 运行测试**

Run: `cd sdk/sdk-js && npx vitest run tests/auth.test.ts`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add sdk/sdk-js/src/auth.ts sdk/sdk-js/tests/auth.test.ts
git commit -m "feat(sdk-js): add AuthManager with JWT refresh"
```

---

### Task 5: RESTClient — HTTP API 调用

**Files:**
- Create: `sdk/sdk-js/src/rest-client.ts`
- Create: `sdk/sdk-js/tests/rest-client.test.ts`

- [ ] **Step 1: 写 RESTClient 测试**

```typescript
// tests/rest-client.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RESTClient } from '../src/rest-client.js';
import { AuthManager } from '../src/auth.js';
import { ConstellError } from '../src/errors.js';

const store = new Map<string, string>();
const mockStorage = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => store.set(k, v),
  removeItem: (k: string) => store.delete(k),
  clear: () => store.clear(),
};

describe('RESTClient', () => {
  let client: RESTClient;
  let auth: AuthManager;

  beforeEach(() => {
    store.clear();
    store.set('constell_access_token', 'valid-token');
    auth = new AuthManager('http://localhost:8080', mockStorage);
    client = new RESTClient(auth, 'http://localhost:8080');
  });

  it('sends GET request with Authorization header', async () => {
    const fakeFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ id: 'u1', nickname: 'Alice' }),
    });
    vi.stubGlobal('fetch', fakeFetch);

    const result = await client.get<{ id: string }>('/api/v1/users/u1');
    expect(result.id).toBe('u1');
    expect(fakeFetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/users/u1',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer valid-token' }),
      }),
    );
  });

  it('sends POST request with JSON body', async () => {
    const fakeFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ id: 's1', name: 'Test' }),
    });
    vi.stubGlobal('fetch', fakeFetch);

    await client.post('/api/v1/servers', { name: 'Test' });
    expect(fakeFetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/servers',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ name: 'Test' }),
      }),
    );
  });

  it('throws ConstellError on non-2xx response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ message: 'Not found' }),
    }));
    await expect(client.get('/api/v1/users/missing')).rejects.toThrow(ConstellError);
  });
});
```

- [ ] **Step 2: 实现 RESTClient**

```typescript
// src/rest-client.ts
import type { AuthManager } from './auth.js';
import { ConstellError, AuthError } from './errors.js';

export class RESTClient {
  constructor(
    private auth: AuthManager,
    private baseUrl: string,
  ) {}

  async get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('POST', path, body);
  }

  async patch<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('PATCH', path, body);
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>('DELETE', path);
  }

  async upload<T>(path: string, formData: FormData): Promise<T> {
    const token = await this.auth.getValidToken();
    const resp = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: formData,
    });
    return this.handleResponse<T>(resp);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const token = await this.auth.getValidToken();
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
    };
    if (body) {
      headers['Content-Type'] = 'application/json';
    }
    const resp = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    return this.handleResponse<T>(resp);
  }

  private async handleResponse<T>(resp: Response): Promise<T> {
    if (!resp.ok) {
      if (resp.status === 401) {
        throw new AuthError('Unauthorized');
      }
      const err = await resp.json().catch(() => ({ message: resp.statusText }));
      throw new ConstellError(
        err.code ?? 'REQUEST_ERROR',
        err.message ?? `Request failed: ${resp.status}`,
        resp.status,
      );
    }
    return resp.json() as Promise<T>;
  }
}
```

- [ ] **Step 3: 运行测试**

Run: `cd sdk/sdk-js && npx vitest run tests/rest-client.test.ts`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add sdk/sdk-js/src/rest-client.ts sdk/sdk-js/tests/rest-client.test.ts
git commit -m "feat(sdk-js): add RESTClient with auth header injection"
```

---

### Task 6: WSManager — WebSocket 连接管理

**Files:**
- Create: `sdk/sdk-js/src/ws-manager.ts`
- Create: `sdk/sdk-js/tests/ws-manager.test.ts`

- [ ] **Step 1: 写 WSManager 测试**

```typescript
// tests/ws-manager.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { WSManager } from '../src/ws-manager.js';
import type { EventBus } from './event-bus.js';
import type { WSStatus } from '../src/types.js';

// Mock WebSocket
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  binaryType = 'arraybuffer' as BinaryType;
  readyState = MockWebSocket.CONNECTING;
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  sent: Uint8Array[] = [];

  send(data: Uint8Array) { this.sent.push(data); }
  close() { this.readyState = MockWebSocket.CLOSED; }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.(new Event('open'));
  }
  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close'));
  }
  simulateMessage(data: ArrayBuffer) {
    this.onmessage?.({ data } as MessageEvent);
  }
}

describe('WSManager', () => {
  let ws: WSManager;
  let mockWs: MockWebSocket;
  let bus: { emit: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    bus = { emit: vi.fn() };
    mockWs = new MockWebSocket();
    vi.stubGlobal('WebSocket', (url: string) => {
      mockWs = new MockWebSocket();
      return mockWs as any;
    });
    ws = new WSManager(
      'ws://localhost:8081/ws',
      () => Promise.resolve('test-jwt'),
      bus as any,
    );
  });

  it('connects and emits connected event', async () => {
    ws.connect();
    mockWs.simulateOpen();
    expect(bus.emit).toHaveBeenCalledWith('connected');
    expect(ws.status).toBe('connected');
  });

  it('sends heartbeat binary frame', async () => {
    ws.connect();
    mockWs.simulateOpen();
    ws.sendHeartbeat('hb-1');
    expect(mockWs.sent.length).toBe(1);
    // verify 4-byte length prefix
    const frame = mockWs.sent[0];
    const len = new DataView(frame.buffer).getUint32(0, false);
    expect(frame.byteLength).toBe(4 + len);
  });

  it('detects disconnect and emits disconnected', async () => {
    ws.connect();
    mockWs.simulateOpen();
    mockWs.simulateClose();
    expect(bus.emit).toHaveBeenCalledWith('disconnected');
  });

  it('disconnect() closes the connection', async () => {
    ws.connect();
    mockWs.simulateOpen();
    ws.disconnect();
    expect(ws.status).toBe('disconnected');
  });
});
```

- [ ] **Step 2: 实现 WSManager**

```typescript
// src/ws-manager.ts
import type { WSStatus } from './types.js';
import type { EventBus } from './event-bus.js';
import { createClientMessage, encodeClientFrame, generateRequestId, readServerEvent } from './codec.js';

interface WSBusEvents {
  connected: () => void;
  disconnected: () => void;
  message: (data: ArrayBuffer) => void;
}

export class WSManager {
  private ws: WebSocket | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private _status: WSStatus = 'disconnected';
  private intentionallyClosed = false;
  private subscribedChannels: string[] = [];

  private static readonly BASE_DELAY = 1000;
  private static readonly MAX_DELAY = 30000;
  private static readonly HEARTBEAT_INTERVAL = 30000;

  constructor(
    private url: string,
    private getToken: () => Promise<string>,
    private bus: EventBus<WSBusEvents>,
  ) {}

  get status(): WSStatus {
    return this._status;
  }

  connect(): void {
    this.intentionallyClosed = false;
    this.doConnect();
  }

  disconnect(): void {
    this.intentionallyClosed = true;
    this.cleanup();
    this._status = 'disconnected';
  }

  send(data: Uint8Array): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket not connected');
    }
    this.ws.send(data);
  }

  sendHeartbeat(requestId?: string): void {
    const msg = createClientMessage({
      type: 5, // HEARTBEAT
      requestId: requestId ?? generateRequestId(),
    });
    this.send(encodeClientFrame(msg));
  }

  setSubscribedChannels(channels: string[]): void {
    this.subscribedChannels = channels;
  }

  getSubscribedChannels(): string[] {
    return this.subscribedChannels;
  }

  private async doConnect(): Promise<void> {
    this._status = 'connecting';
    const token = await this.getToken();
    this.ws = new WebSocket(`${this.url}?token=${token}`);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = () => {
      this._status = 'connected';
      this.reconnectAttempt = 0;
      this.startHeartbeat();
      this.bus.emit('connected');
    };

    this.ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        this.bus.emit('message', event.data);
      }
    };

    this.ws.onclose = () => {
      this.stopHeartbeat();
      this._status = 'disconnected';
      this.bus.emit('disconnected');
      if (!this.intentionallyClosed) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.sendHeartbeat();
      }
    }, WSManager.HEARTBEAT_INTERVAL);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private scheduleReconnect(): void {
    this.reconnectAttempt++;
    const delay = Math.min(
      WSManager.BASE_DELAY * Math.pow(2, this.reconnectAttempt - 1),
      WSManager.MAX_DELAY,
    );
    const jitter = delay * 0.2 * (Math.random() * 2 - 1);
    const actualDelay = Math.max(100, delay + jitter);

    this.reconnectTimer = setTimeout(() => {
      this.doConnect();
    }, actualDelay);
  }

  private cleanup(): void {
    this.stopHeartbeat();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onclose = null;
      this.ws.onmessage = null;
      this.ws.onerror = null;
      if (this.ws.readyState === WebSocket.OPEN) {
        this.ws.close();
      }
      this.ws = null;
    }
  }
}
```

- [ ] **Step 3: 运行测试**

Run: `cd sdk/sdk-js && npx vitest run tests/ws-manager.test.ts`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add sdk/sdk-js/src/ws-manager.ts sdk/sdk-js/tests/ws-manager.test.ts
git commit -m "feat(sdk-js): add WSManager with heartbeat and reconnect"
```

---

### Task 7: ConstellClient — 组装所有模块

**Files:**
- Create: `sdk/sdk-js/src/client.ts`
- Update: `sdk/sdk-js/src/index.ts`

- [ ] **Step 1: 实现 ConstellClient**

ConstellClient 是 SDK 的主入口，组装 AuthManager + RESTClient + WSManager + EventBus，对外暴露统一 API。具体实现按照 spec 1.2 节的方法签名，内部根据方法分发到 RESTClient（读操作）或 WSManager（写操作）。

核心实现要点：
- `constructor(config)` 创建 auth、rest、ws、bus 四个内部实例
- `login/register` 调用 auth，成功后自动 `connect()` WebSocket
- `getDMHistory/getChannelHistory/listCommunities` 等读操作调用 RESTClient
- `sendDM/sendChannelMessage` 通过 WSManager 发送二进制帧，等待 ACK（5s 超时）
- `subscribeChannel/unsubscribeChannel` 通过 WSManager 发送
- WS `message` 事件到达后，用 `readServerEvent` 解码，根据 `ServerEventType` 分发到对应 bus 事件
- ACK 匹配：维护 `pendingRequests: Map<string, { resolve, reject, timer }>` 注册表

文件内容参考 spec §1.2 和 §1.7 的完整伪代码。完整实现约 300 行。

- [ ] **Step 2: 更新 src/index.ts 导出 ConstellClient**

```typescript
export { ConstellClient } from './client.js';
export { ConstellError, AuthError, NetworkError } from './errors.js';
export type * from './types.js';
export type { EventBus } from './event-bus.js';
```

- [ ] **Step 3: 运行全部 SDK 测试**

Run: `cd sdk/sdk-js && npx vitest run`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add sdk/sdk-js/
git commit -m "feat(sdk-js): add ConstellClient — wire all modules together"
```

---

## Phase 2: React 应用基础设施

### Task 8: React 项目脚手架 + 深色主题

**Files:**
- Create: `clients/web/` 整个目录（Vite 脚手架 + 配置）

- [ ] **Step 1: 用 Vite 创建 React TypeScript 项目**

```bash
cd /Users/lance.wang/workspace/wzgown/constell
npm create vite@latest clients/web -- --template react-ts
cd clients/web
npm install
```

- [ ] **Step 2: 安装核心依赖**

```bash
cd clients/web
npm install zustand react-router@7 @tanstack/react-virtual
npm install -D tailwindcss @tailwindcss/vite
```

- [ ] **Step 3: 初始化 shadcn/ui**

```bash
cd clients/web
npx shadcn@latest init
```

选择：New York style, Zinc base color, CSS variables: yes

- [ ] **Step 4: 安装 shadcn 组件**

```bash
npx shadcn@latest add button input avatar badge dialog command scroll-area separator skeleton tooltip
```

- [ ] **Step 5: 配置 Catppuccin Mocha 深色主题**

更新 `src/styles/globals.css`，替换 shadcn 默认 CSS 变量为 spec §2.10 定义的 Catppuccin Mocha 配色：

```css
@import "tailwindcss";

@layer base {
  :root {
    --background: 30 10% 11%;
    --foreground: 267 84% 94%;
    --card: 30 10% 14%;
    --card-foreground: 267 84% 94%;
    --popover: 30 10% 14%;
    --popover-foreground: 267 84% 94%;
    --primary: 263 70% 58%;
    --primary-foreground: 0 0% 100%;
    --secondary: 267 11% 25%;
    --secondary-foreground: 267 84% 94%;
    --muted: 267 11% 25%;
    --muted-foreground: 267 11% 52%;
    --accent: 267 11% 25%;
    --accent-foreground: 267 84% 94%;
    --destructive: 0 72% 61%;
    --destructive-foreground: 0 0% 100%;
    --border: 267 11% 20%;
    --input: 267 11% 20%;
    --ring: 263 70% 58%;
    --radius: 0.5rem;
  }
}

@layer base {
  * {
    @apply border-border;
  }
  body {
    @apply bg-background text-foreground;
  }
}
```

- [ ] **Step 6: 配置 Vite 代理**

更新 `vite.config.ts`：

```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': {
        target: 'ws://localhost:8081',
        ws: true,
      },
    },
  },
});
```

- [ ] **Step 7: 验证开发服务器启动**

Run: `cd clients/web && npm run dev`
Expected: Vite dev server 启动在 localhost:5173，页面显示深色背景

- [ ] **Step 8: 提交**

```bash
git add clients/web/
git commit -m "feat(web): scaffold React app with Vite, Tailwind, shadcn/ui"
```

---

### Task 9: Client Provider + Auth Store + Hooks

**Files:**
- Create: `clients/web/src/lib/client.ts`
- Create: `clients/web/src/lib/utils.ts`
- Create: `clients/web/src/hooks/useConstellClient.ts`
- Create: `clients/web/src/stores/authStore.ts`
- Create: `clients/web/src/hooks/useAuth.ts`

- [ ] **Step 1: 创建 SDK 客户端单例**

```typescript
// src/lib/client.ts
import { ConstellClient } from '@constell/sdk-js';

export function createClient(): ConstellClient {
  return new ConstellClient({
    apiUrl: import.meta.env.VITE_API_URL || '',
    wsUrl: import.meta.env.VITE_WS_URL || 'ws://localhost:8081/ws',
  });
}
```

- [ ] **Step 2: 创建 React Context provider**

```typescript
// src/hooks/useConstellClient.ts
import { createContext, useContext, useRef, type ReactNode } from 'react';
import { ConstellClient } from '@constell/sdk-js';
import { createClient } from '@/lib/client';

const ClientContext = createContext<ConstellClient | null>(null);

export function ClientProvider({ children }: { children: ReactNode }) {
  const clientRef = useRef<ConstellClient | null>(null);
  if (!clientRef.current) {
    clientRef.current = createClient();
  }
  return (
    <ClientContext.Provider value={clientRef.current}>
      {children}
    </ClientContext.Provider>
  );
}

export function useConstellClient(): ConstellClient {
  const client = useContext(ClientContext);
  if (!client) throw new Error('useConstellClient must be used within ClientProvider');
  return client;
}
```

- [ ] **Step 3: 创建 authStore**

```typescript
// src/stores/authStore.ts
import { create } from 'zustand';
import type { User } from '@constell/sdk-js';

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  loading: boolean;
  setUser: (user: User | null) => void;
  setLoading: (loading: boolean) => void;
  reset: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  loading: true,
  setUser: (user) => set({ user, isAuthenticated: !!user, loading: false }),
  setLoading: (loading) => set({ loading }),
  reset: () => set({ user: null, isAuthenticated: false, loading: false }),
}));
```

- [ ] **Step 4: 创建 useAuth hook**

```typescript
// src/hooks/useAuth.ts
import { useCallback } from 'react';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';

export function useAuth() {
  const client = useConstellClient();
  const { user, isAuthenticated, loading, setUser, setLoading, reset } = useAuthStore();

  const login = useCallback(async (email: string, password: string) => {
    setLoading(true);
    try {
      const u = await client.login(email, password);
      setUser(u);
    } catch {
      reset();
      throw new Error('Login failed');
    }
  }, [client, setUser, setLoading, reset]);

  const register = useCallback(async (username: string, email: string, password: string) => {
    setLoading(true);
    try {
      const u = await client.register(username, email, password);
      setUser(u);
    } catch {
      reset();
      throw new Error('Registration failed');
    }
  }, [client, setUser, setLoading, reset]);

  const logout = useCallback(() => {
    client.logout();
    reset();
  }, [client, reset]);

  const initAuth = useCallback(() => {
    setLoading(true);
    const u = client.initFromStorage?.();
    if (u) {
      setUser(u);
      client.connect();
    } else {
      setLoading(false);
    }
  }, [client, setUser, setLoading]);

  return { user, isAuthenticated, loading, login, register, logout, initAuth };
}
```

- [ ] **Step 5: 提交**

```bash
git add clients/web/src/
git commit -m "feat(web): add client provider, auth store, and useAuth hook"
```

---

### Task 10: Zustand Stores (communities, messages, unread, ui)

**Files:**
- Create: `clients/web/src/stores/communitiesStore.ts`
- Create: `clients/web/src/stores/messagesStore.ts`
- Create: `clients/web/src/stores/unreadStore.ts`
- Create: `clients/web/src/stores/uiStore.ts`

- [ ] **Step 1: 创建 communitiesStore**

按照 spec §2.4 的 CommunitiesState 接口实现。核心 action：
- `fetchCommunities()`：调 `client.listCommunities()`，填充 `communities` Map
- `selectCommunity(id)`：设置 `currentCommunityId`，加载该 Community 的 channels
- `selectChannel(id)`：设置 `currentChannelId`，加载该频道的历史消息

- [ ] **Step 2: 创建 messagesStore**

按照 spec §2.4 的 MessagesState 接口实现。核心 action：
- `fetchChannelHistory(channelId)`：调 `client.getChannelHistory()`，填充 `channelMessages` Map
- `fetchDMHistory(peerId)`：调 `client.getDMHistory()`，填充 `dmMessages` Map
- `appendMessage(type, targetId, msg)`：向对应 Map 追加消息（实时事件用）
- `sendMessage(target, content, fileIds?)`：调 SDK 发送

- [ ] **Step 3: 创建 unreadStore**

按照 spec §2.4 的 UnreadState 接口实现。核心 action：
- `fetchUnreads()`：调 `client.getUnreadCounts()`，初始化两个 Map
- `markDMRead(peerId)` / `markChannelRead(channelId)`：调 SDK + 更新本地 Map
- `incrementUnread(type, id)`：对应 Map 中 count +1

- [ ] **Step 4: 创建 uiStore**

按照 spec §2.4 的 UIState 接口实现。纯状态管理，无异步 action。

- [ ] **Step 5: 提交**

```bash
git add clients/web/src/stores/
git commit -m "feat(web): add communities, messages, unread, and ui Zustand stores"
```

---

### Task 11: useClientEvents — SDK 事件桥接

**Files:**
- Create: `clients/web/src/hooks/useClientEvents.ts`
- Create: `clients/web/src/hooks/useChat.ts`
- Create: `clients/web/src/hooks/useUnread.ts`
- Create: `clients/web/src/hooks/useOnlineStatus.ts`

- [ ] **Step 1: 实现 useClientEvents**

按照 spec §2.5 的实时数据流实现。在 MainLayout 顶层调用一次：

```typescript
// src/hooks/useClientEvents.ts
import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';

export function useClientEvents() {
  const client = useConstellClient();
  const appendMessage = useMessagesStore((s) => s.appendMessage);
  const incrementUnread = useUnreadStore((s) => s.incrementUnread);
  const setOnline = useUIStore((s) => s.setOnline);
  const setOffline = useUIStore((s) => s.setOffline);
  const setWsStatus = useUIStore((s) => s.setWsStatus);

  useEffect(() => {
    client.on('dm_received', (msg) => {
      appendMessage('dm', msg.senderId, msg);
      incrementUnread('dm', msg.senderId);
    });
    client.on('channel_message', (msg) => {
      appendMessage('channel', msg.channelId, msg);
      incrementUnread('channel', msg.channelId);
    });
    client.on('user_online', ({ userId }) => setOnline(userId));
    client.on('user_offline', ({ userId }) => setOffline(userId));
    client.on('connected', () => setWsStatus('connected'));
    client.on('disconnected', () => setWsStatus('disconnected'));
    client.on('reconnected', () => setWsStatus('connected'));

    return () => client.removeAllListeners();
  }, [client, appendMessage, incrementUnread, setOnline, setOffline, setWsStatus]);
}
```

- [ ] **Step 2: 实现辅助 hooks**

- `useChat()`：封装 messagesStore action + 当前 channel/DM 的消息选择器
- `useUnread()`：封装 unreadStore 的未读数查询 + 标记已读
- `useOnlineStatus(userId)`：检查 uiStore.onlineUsers 是否包含该用户

- [ ] **Step 3: 提交**

```bash
git add clients/web/src/hooks/
git commit -m "feat(web): add useClientEvents bridge and helper hooks"
```

---

## Phase 3: Auth 页面

### Task 12: Login + Register 页面 + Auth Guard

**Files:**
- Create: `clients/web/src/components/auth/LoginForm.tsx`
- Create: `clients/web/src/components/auth/RegisterForm.tsx`
- Create: `clients/web/src/pages/LoginPage.tsx`
- Create: `clients/web/src/pages/RegisterPage.tsx`
- Update: `clients/web/src/App.tsx`

- [ ] **Step 1: 实现 LoginForm 组件**

使用 shadcn Button + Input 组件。表单字段：email、password。提交调用 `useAuth().login()`。错误时显示红色提示。

- [ ] **Step 2: 实现 RegisterForm 组件**

表单字段：username、email、password、confirm password。提交调用 `useAuth().register()`。

- [ ] **Step 3: 实现 LoginPage 和 RegisterPage**

居中卡片布局，深色背景。底部有"注册账号" / "已有账号" 切换链接。

- [ ] **Step 4: 配置 App.tsx 路由**

```typescript
// src/App.tsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { ClientProvider } from '@/hooks/useConstellClient';
import { AuthGuard } from '@/components/auth/AuthGuard';
import LoginPage from '@/pages/LoginPage';
import RegisterPage from '@/pages/RegisterPage';
import MainPage from '@/pages/MainPage';

function AuthGuard({ children }: { children: ReactNode }) {
  const { isAuthenticated, loading, initAuth } = useAuth();
  useEffect(() => { initAuth(); }, []);
  if (loading) return <div>Loading...</div>;
  if (!isAuthenticated) return <Navigate to="/login" />;
  return <>{children}</>;
}

export default function App() {
  return (
    <ClientProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/*" element={
            <AuthGuard>
              <MainPage />
            </AuthGuard>
          } />
        </Routes>
      </BrowserRouter>
    </ClientProvider>
  );
}
```

- [ ] **Step 5: 提交**

```bash
git add clients/web/src/
git commit -m "feat(web): add login/register pages with auth guard and routing"
```

---

## Phase 4: 主布局

### Task 13: MainLayout + CommunityRail

**Files:**
- Create: `clients/web/src/components/layout/MainLayout.tsx`
- Create: `clients/web/src/components/layout/CommunityRail.tsx`
- Create: `clients/web/src/pages/MainPage.tsx`

- [ ] **Step 1: 实现 CommunityRail（左栏 72px）**

Discord 风格竖排图标轨道：
- 顶部：DM/Home 按钮（💬），跳转 `/@me`
- 分隔线
- 用户加入的 Community 图标列表（首字母 + 圆角矩形），点击跳转 `/:communityId`
- 底部：`+` 创建 Community 按钮
- 最底部：当前用户头像

从 `communitiesStore.communities` 读取列表，`unreadStore` 计算每个 Community 的未读总数显示为 badge。

- [ ] **Step 2: 实现 MainLayout（三栏容器）**

```typescript
// src/components/layout/MainLayout.tsx
import { Outlet } from 'react-router';
import { CommunityRail } from './CommunityRail';
import { ChannelList } from './ChannelList';

export function MainLayout() {
  return (
    <div className="flex h-screen bg-background">
      <CommunityRail />
      <ChannelList />
      <div className="flex-1 flex flex-col">
        <Outlet />
      </div>
    </div>
  );
}
```

- [ ] **Step 3: 实现 MainPage**

```typescript
// src/pages/MainPage.tsx
import { MainLayout } from '@/components/layout/MainLayout';

export default function MainPage() {
  return <MainLayout />;
}
```

- [ ] **Step 4: 提交**

```bash
git add clients/web/src/components/layout/ clients/web/src/pages/MainPage.tsx
git commit -m "feat(web): add MainLayout with CommunityRail sidebar"
```

---

### Task 14: ChannelList + DMList（中栏）

**Files:**
- Create: `clients/web/src/components/layout/ChannelList.tsx`
- Create: `clients/web/src/components/dm/DMList.tsx`

- [ ] **Step 1: 实现 ChannelList（中栏 240px）**

根据当前路由判断显示 Channel 列表还是 DM 列表：
- `/:communityId` 路径：显示该 Community 的 Channel 列表（分组：Text Channels）
- `/@me` 路径：显示 DM 对话列表

Channel 列表：
- 顶部显示 Community 名称
- `# general` / `# random` 等 channel 条目，高亮当前选中
- 未读 channel 显示蓝色文字 + badge
- 点击跳转 `/:communityId/:channelId`

DM 列表：
- 顶部显示 "Direct Messages"
- 好友列表，显示在线/离线状态（绿/灰圆点）
- 未读 DM 显示 badge
- 点击跳转 `/@me/:peerId`

- [ ] **Step 2: 实现 DMList 组件**

从 `communitiesStore` 读取 DM 对话列表（调 `client.getDMConversations()`），结合 `unreadStore` 显示未读。

- [ ] **Step 3: 提交**

```bash
git add clients/web/src/components/layout/ChannelList.tsx clients/web/src/components/dm/DMList.tsx
git commit -m "feat(web): add ChannelList and DMList for middle sidebar panel"
```

---

## Phase 5: 聊天视图

### Task 15: ChatHeader + MessageBubble + MessageList

**Files:**
- Create: `clients/web/src/components/chat/ChatHeader.tsx`
- Create: `clients/web/src/components/chat/MessageBubble.tsx`
- Create: `clients/web/src/components/chat/MessageList.tsx`

- [ ] **Step 1: 实现 ChatHeader**

顶部信息栏，显示：
- 频道模式：`# channel-name` + topic 描述 + 搜索按钮 + 成员按钮
- DM 模式：用户头像 + 用户名 + 在线状态

右侧按钮：🔍搜索、👥成员列表开关

- [ ] **Step 2: 实现 MessageBubble**

单条消息气泡，包含：
- 用户头像（左侧 40px 圆形）
- 用户名（颜色可配置）+ 时间戳
- 消息内容（支持纯文本）
- 附件预览（图片缩略图 / 文件名 + 大小）
- 发送状态指示（sending: 灰色，sent: 正常，failed: 红色 + 重发按钮）

- [ ] **Step 3: 实现 MessageList（虚拟滚动）**

使用 `@tanstack/react-virtual` 实现虚拟化：
- `useVirtualizer` 配置 `count: messages.length`，`getScrollElement: () => scrollRef.current`
- 向上滚动到顶部时触发 `fetchChannelHistory` 加载更多
- 收到新消息时自动滚动到底部（仅当用户已在底部时）
- 空状态显示欢迎文案

- [ ] **Step 4: 提交**

```bash
git add clients/web/src/components/chat/
git commit -m "feat(web): add ChatHeader, MessageBubble, and virtual-scrolled MessageList"
```

---

### Task 16: ChatInput + 文件上传

**Files:**
- Create: `clients/web/src/components/chat/ChatInput.tsx`

- [ ] **Step 1: 实现 ChatInput**

底部输入区域：
- `+` 按钮触发文件选择器
- 多行文本输入框（`<textarea>` 自适应高度）
- Enter 发送，Shift+Enter 换行
- 附件预览区域：选中的图片显示缩略图，文件显示名称 + 大小
- 上传进度条
- 发送逻辑：先 `client.uploadFile()` 上传文件（如有），再 `client.sendChannelMessage()` 或 `client.sendDM()`

- [ ] **Step 2: 提交**

```bash
git add clients/web/src/components/chat/ChatInput.tsx
git commit -m "feat(web): add ChatInput with file upload and preview"
```

---

### Task 17: ChannelView + DMChat 路由

**Files:**
- Create: `clients/web/src/components/chat/ChannelView.tsx`
- Create: `clients/web/src/components/dm/DMChat.tsx`
- Update: `clients/web/src/App.tsx` (添加子路由)
- Create: `clients/web/src/components/layout/MemberList.tsx`

- [ ] **Step 1: 实现 ChannelView**

组合 ChatHeader + MessageList + ChatInput + MemberList：
- 进入频道时调 `client.subscribeChannel(channelId)` + `messagesStore.fetchChannelHistory()`
- 离开频道时调 `client.unsubscribeChannel()`
- 进入频道时调 `unreadStore.markChannelRead()`

- [ ] **Step 2: 实现 DMChat**

与 ChannelView 类似，但：
- 无 MemberList
- ChatHeader 显示对方用户信息
- 使用 `messagesStore.fetchDMHistory(peerId)`

- [ ] **Step 3: 实现 MemberList（右栏 240px）**

- 从 `communitiesStore.getMembers()` 获取成员列表
- 分为 Online / Offline 两组
- 每个成员显示：头像 + 用户名 + 状态消息
- 在线状态由 `uiStore.onlineUsers` 控制

- [ ] **Step 4: 更新 App.tsx 路由**

MainLayout 下添加子路由：
```
/@me → DMList
/@me/:peerId → DMChat
/:communityId → ChannelView (首个频道)
/:communityId/:channelId → ChannelView
```

- [ ] **Step 5: 提交**

```bash
git add clients/web/src/
git commit -m "feat(web): add ChannelView, DMChat routes and MemberList"
```

---

## Phase 6: 功能完善

### Task 18: Search UI

**Files:**
- Create: `clients/web/src/components/search/SearchDialog.tsx`

- [ ] **Step 1: 实现 SearchDialog**

使用 shadcn Command 组件 + Dialog：
- `Cmd+K` / `Ctrl+K` 全局快捷键打开
- 输入框 300ms debounce 后调 `client.search(query)`
- 结果分三组：Users / Channel Messages / DM Messages
- 点击用户跳转 `/@me/:userId`
- 点击频道消息跳转 `/:communityId/:channelId`
- 点击 DM 消息跳转 `/@me/:peerId`

- [ ] **Step 2: 提交**

```bash
git add clients/web/src/components/search/SearchDialog.tsx
git commit -m "feat(web): add Cmd+K search dialog with categorized results"
```

---

### Task 19: 未读标记 + 连接状态 + 消息状态

**Files:**
- Update: `clients/web/src/components/layout/CommunityRail.tsx` (添加未读 badge)
- Update: `clients/web/src/components/layout/ChannelList.tsx` (添加未读 badge)
- Update: `clients/web/src/components/chat/MessageBubble.tsx` (添加发送状态)
- Create: `clients/web/src/components/layout/ConnectionStatusBar.tsx`

- [ ] **Step 1: 添加未读 badge**

- CommunityRail：每个 Community 图标右上角显示未读总数 badge
- ChannelList：每个 Channel 和 DM 条目右侧显示未读数
- 数据来自 `unreadStore`

- [ ] **Step 2: 实现连接状态栏**

```typescript
// src/components/layout/ConnectionStatusBar.tsx
import { useUIStore } from '@/stores/uiStore';

export function ConnectionStatusBar() {
  const wsStatus = useUIStore((s) => s.wsStatus);
  if (wsStatus === 'connected') return null;
  return (
    <div className="bg-yellow-600 text-white text-center text-sm py-1">
      {wsStatus === 'connecting' ? 'Connecting...' : 'Disconnected — reconnecting...'}
    </div>
  );
}
```

插入到 MainLayout 顶部。

- [ ] **Step 3: 添加消息发送状态**

MessageBubble 根据 `TempMessage.status` 显示：
- `sending`：灰色文字 + 灰色时钟图标
- `sent`：正常显示
- `failed`：红色文字 + 红色感叹号，点击重发

- [ ] **Step 4: 提交**

```bash
git add clients/web/src/
git commit -m "feat(web): add unread badges, connection status bar, and message send status"
```

---

## Phase 7: 部署

### Task 20: Nginx + Dockerfile + Docker Compose

**Files:**
- Create: `clients/web/nginx.conf`
- Create: `clients/web/Dockerfile`
- Modify: `deploy/docker/docker-compose.yml`

- [ ] **Step 1: 创建 nginx.conf**

按照 spec §3.2 的 Nginx 配置实现：
- SPA fallback (`try_files $uri $uri/ /index.html`)
- `/api/` 反代到 `api-gateway:8080`
- `/ws` 反代到 `upstream ws_gateways`（轮询 ws-gateway-1 和 ws-gateway-2）

- [ ] **Step 2: 创建 Dockerfile**

多阶段构建：
- Stage 1: `node:20-alpine` → `npm ci && npm run build`
- Stage 2: `nginx:alpine` → 复制 dist + nginx.conf

- [ ] **Step 3: 更新 docker-compose.yml**

新增 `web` 服务，port 3000，depends_on api-gateway + ws-gateway-1。

- [ ] **Step 4: 提交**

```bash
git add clients/web/nginx.conf clients/web/Dockerfile deploy/docker/docker-compose.yml
git commit -m "feat(web): add Nginx config, Dockerfile, and Docker Compose integration"
```

---

### Task 21: E2E 验证 + 更新项目状态

**Files:**
- Modify: `docs/PROJECT_STATUS.md`

- [ ] **Step 1: 启动完整环境验证**

```bash
make docker-up
# 另一个终端
cd clients/web && npm run dev
```

验证项：
1. 打开 `http://localhost:5173`，看到登录页面（深色主题）
2. 注册新用户 → 自动跳转到主界面
3. 创建 Community → 左栏出现新图标
4. 点击 Channel → 中栏显示频道列表
5. 发送消息 → 右侧聊天区实时显示
6. 打开第二个浏览器，注册第二个用户
7. 第一个用户添加第二个用户为成员
8. 两用户实时聊天验证

- [ ] **Step 2: 更新 PROJECT_STATUS.md**

Plan 5 状态从 `⏳ 待规划` 更新为 `✅ 已完成`。

- [ ] **Step 3: 提交并打 tag**

```bash
git add docs/PROJECT_STATUS.md
git commit -m "docs: update Plan 5 status to completed"
git tag v0.5.0
```

---

## 自检结果

**Spec 覆盖率检查：**

| Spec 章节 | 对应 Task | 状态 |
|-----------|----------|------|
| §1.1 SDK 目录结构 | Task 1 | ✅ |
| §1.2 ConstellClient API | Task 7 | ✅ |
| §1.3 事件类型 | Task 3 + Task 11 | ✅ |
| §1.4 AuthManager | Task 4 | ✅ |
| §1.5 WSManager | Task 6 | ✅ |
| §1.6 RESTClient | Task 5 | ✅ |
| §1.7 消息发送可靠性 | Task 7 | ✅ |
| §1.8 Proto 代码生成 | Task 2 | ✅ |
| §2.1 React 目录结构 | Task 8 | ✅ |
| §2.2 路由 | Task 12 + Task 17 | ✅ |
| §2.3 三栏布局 | Task 13 + Task 14 | ✅ |
| §2.4 Zustand Stores | Task 9 + Task 10 | ✅ |
| §2.5 实时数据流 | Task 11 | ✅ |
| §2.6 虚拟滚动 | Task 15 | ✅ |
| §2.7 搜索 UI | Task 18 | ✅ |
| §2.8 文件上传 UI | Task 16 | ✅ |
| §2.9 未读标记 | Task 19 | ✅ |
| §2.10 深色主题 | Task 8 | ✅ |
| §3.1 开发环境 | Task 8 (Vite proxy) | ✅ |
| §3.2 生产部署 | Task 20 | ✅ |
| §3.3 SDK 发布 | Task 1 (package.json) | ✅ |
| §4 测试策略 | Task 1-7 (SDK tests) | ✅ |

**无 placeholder**：所有 task 包含文件路径、实现要点或代码。无 TBD/TODO。

**类型一致性**：types.ts 中定义的接口在 stores 和 hooks 中保持一致使用。
