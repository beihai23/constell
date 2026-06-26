# 80 · 文件（上传 / 预览 / 发送）

> 域前缀 `FILE-*`。实现：`components/chat/ChatInput.tsx`(FilePicker/preview/upload)、SDK `uploadFile`。发送链路见 `features/40` A。

## 0. 概述与用户目标

在消息里附带图片或文件，先预览再发送，接收方可查看/下载。用户目标：富消息（图/附件）。

## 1. 屏幕与布局

- ChatInput 左侧 `[+]` 附件键 → 系统文件选择。
- 选中后，输入区上方出现预览行：图片缩略图或文件卡（图标+文件名+大小+✕移除）。
- 发送后，消息气泡内渲染附件（图缩略/文件卡，点击新窗打开 `attachment.url`）。

```
[图1预览 ✕] [文件卡 ✕]            ← pendingFiles 预览行
[+] [Message …             ] [Send]  ← 输入区
```

## 2. 状态矩阵

| 状态 | 触发 | 期望 UI | 数据 | 断言 |
|---|---|---|---|---|
| 选择 | 点 [+] | 系统文件选择器（多选） | — | 选择器打开 |
| 待发预览 | 选了文件 | 预览行：图缩略/文件卡+大小+✕ | `pendingFiles[]` | 预览可见 |
| 移除待发 | 点 ✕ | 该预览消失；object URL revoke | — | 预览消失 |
| 上传中 | 发送触发 uploadFile | （目标）行内进度/禁用；当前无反馈 | uploading | 进度可见 |
| 上传失败 | uploadFile 抛错 | 整条发送失败（toast）；预览保留可重试 | — | toast；预览保留 |
| 发送成功 | uploadFile+send OK | 预览清空；气泡含附件 | `attachments[]` | 气泡见附件 |
| 接收-图片 | 气泡 attachment 为图 | 缩略图，点击新窗打开 | thumbnailUrl | 图片可见 |
| 接收-文件 | 气泡 attachment 为文件 | 文件卡（图标+名+大小），点击下载 | — | 卡片可见 |
| 超大 | 超过上限（目标） | 选择即拒 + 提示 | — | 提示可见 |

## 3. 流程

- **选择**：`[+]`→隐藏 `<input type=file multiple>`→`handleFileSelect` 读取为 `Uint8Array`，图片建 object URL 预览。
- **发送**（见 `40` A3 带附件分支）：逐个 `uploadFile(data,name,type)`→收 `fileId`+url→乐观气泡带 attachment→`sendChannelMessage`/`sendDM` 带 fileIds。
- **接收**：MessageBubble 的 `AttachmentPreview` 按 contentType 渲染图/文件卡。

## 4. 验收标准

```
AC FILE-PICK-1  选择文件生成预览
  GIVEN 用户在有 target 的会话
  WHEN 点 [+] 选择 a.png 和 b.txt
  THEN 预览行出现两项（图缩略 + 文件卡）

AC FILE-REMOVE-1  移除待发文件
  GIVEN 已选待发文件
  WHEN 点该项 ✕
  THEN 该项消失，其余保留

AC FILE-SEND-1  带图发送
  GIVEN 已选 a.png 并输入文本
  WHEN 发送
  THEN 气泡含文本+图片缩略；接收方同样可见

AC FILE-SEND-2  纯附件发送（无文本）
  GIVEN 仅选文件、无文本
  WHEN 发送
  THEN 气泡含附件（Send 按钮在有附件时可用，见 40 A2 空内容）

AC FILE-UPLOAD-1  上传进度可见（目标-待建，当前红）
  GIVEN 选择大文件并发送
  THEN 显示上传进度（当前像卡住）

AC FILE-UPLOAD-2  上传失败可重试（当前部分红）
  GIVEN uploadFile 失败
  THEN 明确提示失败，保留预览可重试（不静默）

AC FILE-SIZE-1  超大拒绝（目标-待建）
  GIVEN 选择超过上限的文件
  THEN 拒绝并提示大小上限

AC FILE-VIEW-1  接收方查看
  GIVEN 收到含图片的消息
  THEN 缩略图可见，点击在新标签打开原图
```

## 5. 边界与约束

- 当前**无前端大小校验**（缺口）。
- 多文件顺序上传；任一失败→整条失败（当前实现）。
- 后端依赖：file-service 的 `UploadFile` 需 `file_id`（见缺口 1，历史 bug）。
- 接收方 url 由 file-service/minio 提供。

## 6. 当前实现缺口

1. **【实现·待核→已修】file_id 上传链路 + 读取链路**：上传本身早已可用（"作弊 6" 早修）。本轮修了**读取链路**（FILE-VIEW-1 才能真正验证）：① `MINIO_BASE_URL` 从内部 `minio:9000` 改为浏览器可达的 `http://localhost:9000` + bucket 设 public-read（DEV）；② `GetMessages` 此前**完全不带 attachments** → 加 `channelMessageWithAttachments`（GetAttachmentsByMessage）；③ attachments 的 filename/content_type/size 此前为空（SendMessage 只存 file_id）→ `GetAttachmentsByMessage` JOIN `file_metadata` 补齐；④ `attachmentResponse` 此前无 `url` → api-gateway 新增 `FILES_PUBLIC_BASE`，按 `{base}/constell/originals/{file_id}` 重建 url。✅ FILE-VIEW-1。
2. **【目标-已建】无上传进度** → **已建**：SDK 新增 `RESTClient.uploadWithProgress`（XHR，带 `onProgress` 分数回调），`uploadFile(data,name,type,onProgress?)` 暴露进度；ChatInput 在 send 的上传阶段把每文件标 `uploading` + 更新 `progress`，FilePreview 渲染 `upload-progress` 进度条。✅ AC FILE-UPLOAD-1。
3. **【实现】上传失败归入整条失败**：~~`ChatInput.tsx:113-125` 顺序上传，一个失败则整条 catch → toast「Failed to send」~~ → **已修**：per-file 上传并行、失败文件保留预览并标红 `upload-error`，中止整条发送让用户 remove/retry。✅ AC FILE-UPLOAD-2。
4. **【目标-已建】无大小校验/上限** → **已建**：`MAX_FILE_SIZE = 25MB`，`handleFileSelect` 超限文件拒绝（toast 提示）不入预览，合法文件仍可加。✅ AC FILE-SIZE-1。
5. **【实现】裸色值**：`ChatInput.tsx`(FilePreview) + `MessageBubble.tsx`(AttachmentPreview) 通篇 hex（`#313244/#585b70/#45475a/#cdd6f4/#f38ba8/#181825/#11111b`）。改 token。
6. **【目标-待建】无拖拽上传 / 粘贴上传** —— 列 future。

## 7. 待定问题

- 文件大小上限值？（建议 25MB，对齐 MinIO/S3 常见配额）
- 是否支持图片墙/视频内联播放？（建议 future）
- 附件是否需要病毒扫描/类型白名单？（安全项，建议 future）
