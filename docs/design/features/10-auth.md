# 10 · 认证与会话

> 域前缀 `AUTH-*`。实现：`components/auth/{LoginForm,RegisterForm,AuthGuard}.tsx`、`hooks/useAuth.ts`、`hooks/useAuthGate.ts`、`stores/authStore.ts`、SDK `auth.ts`。

## 0. 概述与用户目标

注册、登录、会话恢复（token 持久化）、token 刷新、登出。用户目标：快速进入应用；失败有明确反馈；刷新页面不掉登录。

## 1. 屏幕与布局

- `/login`、`/register`：居中卡片（`LoginPage`/`RegisterPage`），内含对应表单。
- AuthGuard：加载中整屏「Loading…」；未登录重定向 `/login`。
- 用户菜单（CommunityRail 底部头像）：昵称 + 邮箱 + Logout。

## 2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| 空表单 | 进入页 | 输入框可用，提交键可点 | — | 输入框可见 |
| 提交中 | 点 Login/Register | 按钮 disabled + 文案「Logging in…」/「Creating account…」 | `loading=true` | 按钮 disabled + 变文案 |
| 成功 | 认证通过 | 跳转 `/@me` | `setUser` | URL=`/@me` |
| 凭证错 | 认证拒绝 | 输入区下方红色错误文案 | `error=msg` | 错误可见 |
| 密码不一致 | 注册两次密码不同 | 红色「Passwords do not match」，不发请求 | — | 错误可见，无网络请求 |
| 会话恢复中 | 打开已登录页 | AuthGuard 整屏「Loading…」 | `authStore.loading` | 加载文案可见 |
| 会话恢复成功 | token 有效 | 进入 `/@me`；后台拉全量 profile | `initFromStorage` + `refreshProfile` | 进入应用 |
| 会话失效 | SDK `unauthorized` | 清状态 → 跳 `/login` | `useAuthGate` reset | URL=`/login` |
| 登出 | 点 Logout | 清状态 → `/login` | `client.logout` + reset | URL=`/login` |

## 3. 流程

- **注册**：填 username/email/password/confirm→前端校验密码一致→`register(username,email,password)`→SDK 发 `{nickname:username,email,password}`（`auth.ts:129`）→成功→setUser→`/@me`。
- **登录**：email/password→`login`→setUser→`/@me`。
- **会话恢复**：`AuthGuard` mount→`initAuth`→`initFromStorage`（token 派生的 user，可能缺 nickname/email）→若有 token 则 setUser + `connect()` + 后台 `refreshProfile` 补全。
- **失效**：SDK 任一请求 401 → 触发 `unauthorized` → `useAuthGate` reset → 跳登录。

## 4. 验收标准

```
AC AUTH-REG-1  注册成功进入应用
  GIVEN 未注册用户在 /register
  WHEN 填写 username/email/password/confirm(一致) 并提交
  THEN 跳转 /@me 且用户已登录

AC AUTH-REG-2  密码不一致被拦
  GIVEN 两次密码不一致
  WHEN 提交
  THEN 显示「Passwords do not match」且不发起注册请求

AC AUTH-REG-3  注册失败有反馈
  GIVEN 邮箱已注册等冲突
  WHEN 提交
  THEN 输入区下方显示服务端错误文案

AC AUTH-LOGIN-1  登录成功进入应用
  GIVEN 已注册用户在 /login
  WHEN 填写 email/password 并提交
  THEN 跳转 /@me

AC AUTH-LOGIN-2  凭证错误有反馈
  GIVEN 密码错误
  WHEN 提交
  THEN 显示错误文案，停留在 /login

AC AUTH-SESS-1  会话恢复（刷新不掉登录）
  GIVEN 用户已登录
  WHEN 整页刷新
  THEN AuthGuard 短暂显示加载态后恢复登录态并进入应用

AC AUTH-SESS-2  失效自动登出
  GIVEN 登录态下 token 失效(401)
  WHEN 触发任意需鉴权请求
  THEN 自动清状态并跳转 /login

AC AUTH-LOGOUT-1  登出回到登录页
  GIVEN 登录态
  WHEN 点用户菜单 Logout
  THEN 跳转 /login 且不能再直接进 /@me
```

## 5. 边界与约束

- 密码最低强度：**当前前端无强度校验**（见缺口）。
- token 存储：localStorage（`constell_access_token`/`constell_refresh_token`），由 SDK 管理。
- `initFromStorage` 派生的 user 缺 nickname/email，靠 `refreshProfile` 补全（存在补全前显示空昵称的窗口）。

## 6. 当前实现缺口

1. **【实现】内联错误用裸色**：`LoginForm.tsx:59`、`RegisterForm.tsx:95` 用 `text-red-500`。**修正**：改 `text-destructive`。
2. **【实现·术语】"Username" 实为 nickname**：表单 label「Username」（`RegisterForm.tsx:41`），但系统概念是展示昵称（SDK 映射为 `nickname`）。**修正**：label 改「Display name」或「Nickname」，避免与未来唯一 handle 混淆；或在 spec 明确「注册即昵称」。
3. **【实现】AuthGuard 加载态过简**：仅文本「Loading…」，无骨架/品牌。**修正**：与全局首屏加载统一（见 `01` §5 缺口 1）。
4. **【目标-待建】无密码强度提示**；无「忘记密码」入口（无后端支持，列 future）。
5. **【实现】refreshProfile 窗口期昵称空**：恢复登录到 profile 返回之间昵称可能为空 → 头像 fallback 用空字符。**修正**：fallback 用 email 首字母或「?」。

## 7. 待定问题

- 是否需要「记住我」/ token 有效期可见？（暂列 future）
- 注册是否要邮箱验证？（暂列 future）
