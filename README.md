<div align="center">

# Lumi

**专为儿童绘本设计的一站式 AI 创作工具**

> 目前处于快速开发阶段，软件和接口可能发生变化，欢迎反馈。

故事自己定 · 角色不跑偏 · 过程可修改 · 作品留在本地

[下载 macOS Apple Silicon / Windows x64 版本](https://github.com/lumikaka/lumi/releases) · [快速开始](#快速开始)

</div>

你可能只是想把孩子睡前冒出的一句话、一次亲子旅行或一段家庭记忆，变成一本真正可以翻看的绘本；也可能已经有了完整故事，想独立完成角色设定、章节、分镜和画面。

真正开始以后，创作常常被拆散在不同工具里：聊天窗口负责想故事，文档负责写正文，文件夹里堆着角色参考图，另一个网站再逐张生成画面。故事改了，分镜没有同步；角色多画几次，样子也越来越陌生。

Lumi 把这些环节放回同一个项目：从一个想法开始，逐步完成剧情与正文、角色和场景设定、画面脚本、页面图片版本、预览和导出。AI 可以帮你向前推进，但故事怎么讲、哪一版留下、哪个画面采用，始终由你决定。

Lumi 会按项目的绘本形式显示创作术语：`project` 称“项目”或“绘本项目”；普通绘本中的 `chapter`、`comic_section`、`storyboard` 分别称“绘本、页面、页面脚本”，条漫中分别称“章节、画面段落、分镜脚本”；`image_variant` 在两种形式下都称“页面图片版本”。数据库、API 和代码继续使用稳定的英文技术标识。

## 为什么是 Lumi

- **不会画，也能开始做绘本**：先说清楚主角是谁、想完成什么、会经历怎样的变化，Lumi 会帮你建立故事和视觉创作的起点。
- **角色和场景有共同参考**：人物、地点和道具集中保存在设定工作区，生成画面时可以带上选中的参考，不必每一张都从头描述。
- **不满意就改，不必推倒重来**：正文、提示词、页面脚本或分镜脚本、页面图片都可以保留多个候选版本，随时切换或恢复。
- **从故事到画面不用来回搬运**：正文可以直接进入画面脚本流程，页面脚本或分镜脚本继续连接设定参考和图片生成。
- **项目是你自己的**：每个绘本项目都保存在你选择的本机文件夹中，可以整体移动、备份和再次打开。

对家长来说，Lumi 把专业绘本制作拆成可以一步步完成的过程，不需要先学会一整套创作软件。对个人创作者来说，它把写作、视觉设定、分镜、出图和版本管理放进同一个工作台，让一个人也能持续推进完整作品。

## 从一个想法到一本绘本

1. **说出故事想法**：从一句话开始，确定主角、愿望、阻碍和变化，也可以直接建立空白项目慢慢写。
2. **完成绘本或章节正文**：普通绘本项目中创建、续写或批量生成绘本；条漫项目中完成对应章节，也可以导入已有的 TXT / Markdown 稿件。
3. **建立角色与场景设定**：整理人物、地点和道具，上传已有图片，或生成新的设定图与图片版本。
4. **把正文拆成画面单元**：普通绘本生成页面与页面脚本；条漫生成画面段落与分镜脚本，再调整顺序、构图、动作、对白和节奏。
5. **生成并挑选画面**：为单个或多个页面或画面段落生成图片，使用设定资产作为参考，并从页面图片版本中选择最终画面。
6. **预览与导出**：按项目形式预览绘本页面或条漫画面段落，确认结果后将单本绘本、单个章节或完整项目导出为原图 ZIP 或 A4 PDF。

每一步都可以手动完成，也可以让 AI 提供初稿。生成结果不会替你做最终决定，你可以继续编辑、重新生成、切换候选或恢复历史版本。

## 两种创作方式

| 方式 | 怎么开始 | 适合谁 |
|---|---|---|
| **YOLO 快速创作** | 输入最小故事创意，自动建立 Story、剧情简介、设定、漫画画面段落和首张图片，再进入工作台继续修改 | 想快速看到故事雏形的家长和创作者 |
| **手动创作** | 从空白项目或已有稿件开始，自己决定何时编辑、导入、生成和采用结果 | 已有明确构思，或希望逐步控制创作过程的个人创作者 |

YOLO 不是不可修改的“一键成品”。它负责把空白页推进到一个可以继续打磨的起点；之后的故事、设定、分镜和图片仍然属于正常项目内容，可以逐项调整。

## 核心能力

### ✍️ 绘本与章节工作台

集中维护剧情简介、绘本或章节正文和创作提示。普通绘本形式使用“绘本”，条漫使用“章节”；两者都可以手动写作、导入、AI 续写或批量规划。正文与剧情简介都会保留版本，外部修改 `STORY.md` 时也不会被静默覆盖。

### 🎭 角色、场景与道具设定

在设定工作区统一整理绘本的视觉基础：整体画风、参考来源、人物、地点和道具。设定图既可以上传，也可以由 AI 生成；同一个设定项可以保留多个图片版本，方便比较和替换。

### 🎬 可编辑的页面脚本与分镜脚本

普通绘本可拆分成多个页面，每页拥有独立的页面脚本和页面图片版本；条漫章节可拆分成多个画面段落，每段拥有独立的分镜脚本和页面图片版本。两种形式都支持调整标题、顺序、镜头、动作与对白、批量出图及可恢复快照。

### 🧩 带参考的图片生成

生成绘本画面时，Lumi 可以从项目设定中选择相关人物、场景和道具，整理成参考拼图再交给图片模型。生成结果按候选版本保存，不会直接抹掉当前采用的画面。

### 💬 项目内 AI 助手

围绕当前项目讨论故事、设定或具体分镜，不必在独立聊天窗口里反复粘贴背景。普通讨论、单个设定项生成和设定引用各有明确范围；执行中的任务、工具调用和等待输入状态都会保留在项目里。

### 🛠️ 提示词与版本由你掌控

故事、章节、设定、分镜和图片生成使用的提示词可以在项目内查看、修改、保存候选和恢复默认值。正文、分镜、图片、提示词与章节快照都保留历史，让尝试新方向不再意味着失去上一版。

### 📂 本地优先，随时带走

每个项目都是一个自包含的本机文件夹，故事、章节、设定、图片和任务记录不会被锁在远端账户里。项目可以整体移动、复制和备份；Lumi 只记录最近打开的位置，不会把完整项目上传到自己的云端。

使用 AI 功能时，完成生成所需的文本和参考图片会发送给你选择的模型服务商。服务商密钥通过操作系统安全存储保护，不会写入项目文件。

### ⏳ 后台执行，刷新后继续

章节、设定、分镜、图片和导出任务在后台执行。刷新页面、短暂断线或重新打开项目后，Lumi 会从项目记录中恢复任务状态；失败任务可以查看原因、取消或重试。

## 快速开始

> [!IMPORTANT]
> 当前桌面安装包支持 macOS Apple Silicon 和 Windows x64。AI 功能需要自行配置阿里云百炼或 Cloudflare AI Gateway；对应模型服务可能产生费用。

1. 前往 [GitHub Releases](https://github.com/lumikaka/lumi/releases)，macOS Apple Silicon 下载 `Lumi-macos-aarch64.app.zip`，Windows x64 下载 `Lumi-windows-x64-setup.exe`。
2. macOS 解压后打开 `Lumi.app`；Windows 运行安装程序后从开始菜单打开 Lumi。macOS 版本尚未公证，Windows 版本没有 Authenticode 签名；如果系统拦截首次启动，请先确认文件来自本仓库 Release 并核对 SHA-256，再决定是否继续。
3. 按首次启动引导连接阿里云百炼或 Cloudflare AI Gateway。
4. 选择“YOLO 快速创作”，输入最小故事创意；或者选择“手动创建”，从空白项目开始。
5. 在剧情、章节、设定和漫画工作台中继续修改，完成后从导出页面生成原图 ZIP 或 A4 PDF。

<details>

<summary><strong>开发与技术细节</strong></summary>

### 技术栈

- 后端：Go 1.25、Echo v4、GORM v2、纯 Go SQLite
- 前端：React 19、Vite 7、React Router 7、TanStack Query 5、Sass、pnpm；管理端使用 MUI + Emotion
- 数据迁移：`github.com/golang-migrate/migrate/v4`
- 后台任务：River Queue `v0.41.0` 与官方 `riverdriver/riversqlite`

### 目录结构

```text
.
├── cmd/
│   ├── lumi_web/          # API、开发前端代理与生产内嵌前端
│   └── lumi_ctl/          # SQLite migration 管理命令
├── db/migrations/
│   ├── app/               # 全局应用库 migration
│   └── project/           # 自包含项目库 migration
├── docs/
│   ├── prds/              # 产品文档根索引
│   └── skills/manage-prd/ # 仓库 PRD Skill
├── internal/
│   ├── config/            # 环境变量配置
│   ├── appstore/          # 全局应用数据与最近项目索引
│   ├── database/          # GORM / SQLite 连接与在线备份
│   ├── dbmigrate/         # golang-migrate 接线
│   ├── httpapi/           # API handler 与统一响应
│   ├── provider/          # 全局 Provider 与可替换的系统密钥存储
│   ├── llm/               # Cloudflare AI Gateway / 百炼文本模型客户端
│   ├── jobqueue/          # 项目级 River client、产品任务与 workers
│   ├── files/             # 项目 Asset Store、恢复提交、扫描、缩略图与 GC
│   ├── project/           # ProjectManager、ProjectStore、锁与路径边界
│   ├── realtime/          # /api/v1/ws Channel hub 与项目 topic 授权
│   ├── server/            # Echo 路由与中间件
│   └── webui/             # Go→Vite 代理与生产静态服务
├── web/                   # React 用户端与管理端
└── bin/tmux               # 双 pane 开发环境
```

### 本地开发

需要 Go 1.25+、Node.js 26+、pnpm 11.16+；一键开发额外需要 tmux。

```bash
cp .env.example .env
make deps
./bin/tmux
```

也可以分别运行：

```bash
make dev-web
make dev-api
```

开发环境只从 Go Server 访问：

| 端口 | 服务 |
|---|---|
| `5801` | Go API、用户端与管理端统一入口 |
| `5802` | Vite 开发服务器，由 Go 反向代理 |

- 用户端：<http://localhost:5801/>
- 管理端：<http://localhost:5801/admin>
- 健康检查：<http://localhost:5801/api/v1/health>
- 应用 WebSocket：<ws://localhost:5801/api/v1/ws>

Go 会优先处理 `/api`；开发环境的其他前端请求和 Vite HMR WebSocket 会转发到 `5802`。Vite 未运行时，前端请求返回 `502 Bad Gateway`，API 仍可使用。

应用实时通信采用 `topic/event/payload/ref/join_ref` Channel 信封，提供 join、leave、heartbeat、自动重连和按 topic 广播。`system` 用于应用状态；同一连接可以同时订阅任意已打开项目的 `project:<project_uuid>`，每个 topic 独立持有 Presence lease。WebSocket 只负责即时提示，业务状态不使用定时 HTTP 轮询；客户端在首次 join、重新 join 和窗口重新聚焦时通过 REST task/event cursor 校准 SQLite 事实状态。若项目已被空闲回收，URL 会先幂等重开再重新 join。

### 配置

服务端读取环境变量，并在开发环境自动加载项目根目录的 `.env`：

| 变量 | 默认值 |
|---|---|
| `APP_ENV` | `development` |
| `LOG_LEVEL` | development/test 为 `info`，production 为 `warn`；可设为 `debug`、`info`、`warn` 或 `error` |
| `APP_ADDRESS` | `127.0.0.1:5801`，默认只监听 loopback |
| `FRONTEND_URL` | `http://localhost:5801` |
| `VITE_DEV_SERVER_URL` | `http://127.0.0.1:5802` |
| `LUMI_DATA_DIR` | macOS/Linux：development 为 `~/.lumi_dev`、production 为 `~/.lumi`；Windows：development 为 `%LOCALAPPDATA%\dev.lumi.Lumi-dev`、production 为 `%LOCALAPPDATA%\dev.lumi.Lumi` |
| `DATABASE_DSN` | 可选测试覆盖；默认是 `{LUMI_DATA_DIR}/lumi.sqlite`，启用 foreign keys、WAL、NORMAL synchronous 与 5 秒 busy timeout |

HTTP 请求按响应状态分级：1xx–3xx 为 Info，4xx 为 Warning，5xx 为 Error；显式设置 `LOG_LEVEL` 会覆盖环境默认值，无效值会阻止服务启动。

应用数据目录只包含全局 `lumi.sqlite` 与可删除的 `cache/`。`lumi.sqlite` 保存 Provider 的非敏感配置和加密 secret envelope；Cloudflare API Token / 百炼 API Key 使用操作系统 Keychain 中的根密钥加密。章节、故事、设定、资产和项目任务不得进入全局库。

### 项目目录与数据边界

新建项目的父目录默认为系统 Documents 目录下的 `Lumi`：macOS/Linux 通常为 `~/Documents/Lumi/`，Windows 使用系统 Known Folder，因此会跟随 OneDrive 或企业策略重定向。首次创建项目时，Lumi 会自动创建该目录；仍可在新建弹窗中改用其他已经存在的绝对目录。

创建项目会生成：

```text
{project_root}/
├── README.md
├── STORY.md
├── project.sqlite
├── assets/
└── .lumi/
    ├── cache/
    ├── thumbnails/
    ├── tmp/
    ├── quarantine/
    └── backups/
```

`project.sqlite` 是结构化数据事实源。项目内长期数据只能保存相对项目根目录的规范化路径；`.lumi/project.lock` 是进程级写锁元数据，进程崩溃后操作系统会自动释放实际锁。项目 migration 前使用 SQLite backup API 在 `.lumi/backups/` 创建一致性备份。

每个已打开项目拥有独立 River client。River 使用该项目 `project.sqlite` 的单连接 `*sql.DB`，有数据库副作用的 queue 保持每项目 `MaxWorkers=1`。项目 migration 与备份完成后才执行官方 River migration；打开另一个项目不会停止现有 worker，只有目标项目回收/关闭或应用退出时才先 soft-stop 对应 worker、再关闭数据库。完整边界与升级 gate 见 [AI 运行时说明](docs/ai-runtime.md)。

上传、AI 生成图片和长期派生资产先进入 `.lumi/tmp/{uuid}.part`，通过服务端 purpose allowlist、真实 MIME、媒体解码、大小/像素限制和 SHA-256 校验后，提交为 `file_objects` 物理对象与 `files` 逻辑 Asset。业务 API 只使用 Asset UUID 和 `content_url`，二进制由 UUID 解析的 `/media` 路由读取；`assets/` 从不作为静态目录暴露。漫画导出 ZIP/PDF 是短期例外：它只写入项目根 `exports/`，通过 Export UUID 下载并固定保留 7 天，不进入 Asset Store。完整生命周期见 [Asset Store 规范](docs/asset-storage.md)。

章节正文、Story Profile 和 Prompt 候选采用 append-only 版本。`STORY.md` 是 current Story Profile 的人类可读投影：正常保存使用临时文件与原子 rename；外部修改会进入显式冲突状态，只能由用户选择“导入为新版本”或“以数据库版本重新生成”。

API 成功响应：

```json
{ "success": true, "data": { "status": "ok", "database": "connected" } }
```

API 失败响应：

```json
{ "success": false, "data": null, "error": { "code": "...", "message": "...", "details": "..." } }
```

### Database migration

`lumi_ctl` 内嵌两套独立 migration，因此新增 SQL 后需要重新构建二进制。应用库可由 CLI 管理；项目库升级必须经过项目锁与一致性备份协议。

```bash
make migrate-new scope=app name=create_example
make migrate-new scope=project name=create_example
make migrate-up
make migrate-version
make migrate-down steps=1
```

也可以直接调用：

```bash
go run ./cmd/lumi_ctl migrate create app create_example
go run ./cmd/lumi_ctl migrate create project create_example
go run ./cmd/lumi_ctl migrate app up
go run ./cmd/lumi_ctl migrate app down 1
go run ./cmd/lumi_ctl migrate app version
go run ./cmd/lumi_ctl migrate project up /absolute/path/to/project
go run ./cmd/lumi_ctl migrate project version /absolute/path/to/project
```

文件名使用 UTC 时间戳与小写 snake_case。golang-migrate 会为 SQLite migration 自动包裹事务，不要在 SQL 文件中添加 `BEGIN` 或 `COMMIT`。

### 测试与构建

```bash
make test
make build
make build-linux
```

当前平台产物位于 `build/lumi_web` 和 `build/lumi_ctl`。Linux amd64 产物位于 `build/linux-amd64/`。纯 Go SQLite 驱动允许在没有 C 工具链时使用 `CGO_ENABLED=0` 交叉编译。

正式构建会把 `web/dist` 嵌入 `lumi_web`，运行时不需要单独部署前端目录。`APP_ENV=production` 时如果二进制未使用 `embed_frontend` 标签构建，服务会拒绝启动。

### 桌面安装包

Lumi 的 macOS 与 Windows 版本保持 `browser → Go backend` 架构。Tauri 2 只作为桌面启动器和系统托盘运行，不创建 WebView 主窗口：启动器选择随机 loopback 端口和本次运行专用的随机访问令牌，运行安装包内的 `lumi_web`，等待 `/api/v1/health` 确认数据库可用，然后用系统默认浏览器建立桌面会话并打开 Lumi。REST、WebSocket 和媒体内容都需要该桌面会话；退出托盘应用会同时终止 Go 子进程并使已有会话失效。普通开发服务器和未由 Tauri 提供令牌的 `lumi_web` 不启用这一机制。

正式 Release 构建启用 Tauri Updater。应用启动时会后台检查 stable 更新，发现新版本后询问是否下载并安装；也可以从托盘菜单选择 **Check for Updates…** 手动检查。更新包包含 Tauri 启动器、前端和 Go 后端，安装前会使用内置公钥验证 Tauri updater 签名，完成后终止旧后端并重启应用。数据库和用户数据保存在平台应用数据目录中（macOS 为 `~/.lumi`，Windows 为 `%LOCALAPPDATA%\dev.lumi.Lumi`），不属于应用更新包。普通本地构建和 pull request 构建不启用 updater，也不会访问 Release 更新通道。

令牌只存在于 Tauri、Go 进程和浏览器的 HttpOnly 会话 Cookie 中，不写入数据库、Keychain 或日志。菜单中的 **Copy Access URL** 会复制可在另一浏览器会话中建立访问权限的完整 URL，应按密码对待且不要分享。应用重启后，旧页面会提示从菜单栏重新打开。桌面令牌只用于保护 loopback 服务；如果未来把 Lumi 监听到非 loopback 地址，仍必须使用 TLS 和正式的用户认证。

#### macOS Apple Silicon 本地构建

macOS 本地桌面构建只支持 Apple Silicon（`aarch64-apple-darwin`），需要：

- macOS Apple Silicon 与 Xcode Command Line Tools
- Go 1.25
- Node.js 26、pnpm 11.16
- Rust stable
- Tauri CLI 2.8.0：`cargo install tauri-cli --version '=2.8.0' --locked`

构建或构建后启动应用：

```bash
make desktop-build
make desktop-app
```

包含 Rust 测试、bundle 内容和签名校验的完整本地检查为：

```bash
make desktop-check
```

生成的应用位于：

```text
rel/app/src-tauri/target/aarch64-apple-darwin/release/bundle/macos/Lumi.app
```

首次启动可在 Finder 中打开 `Lumi.app`，或运行 `open rel/app/src-tauri/target/aarch64-apple-darwin/release/bundle/macos/Lumi.app`。启动后可以从菜单栏的 Lumi 托盘菜单重新打开页面、复制 Access URL、查看 `~/Library/Logs/Lumi/lumi.log` 或退出应用。release 包默认只记录 Warning/Error；日志达到 5 MiB 后轮转并保留 `lumi.log.1`、`lumi.log.2` 两个备份。生产数据仍保存在 `~/.lumi`；安装包不改变项目目录或数据库格式。

从 GitHub Release 下载时，解压 `Lumi-macos-aarch64.app.zip` 后得到的真正应用是 `Lumi.app`。从 Actions 页面下载的 `Lumi-macos-aarch64` workflow artifact 是一个容器，里面还有 `Lumi-macos-aarch64.app.zip` 和 SHA-256 文件，需要再解压内层 ZIP；不要把 artifact 容器目录改名为或当作 `.app` 打开。如果确认文件来自本仓库 Release 且 SHA-256 一致，但 macOS 仍因未公证而阻止 `Lumi.app`，请在首次尝试打开后前往“系统设置 → 隐私与安全性”，使用“仍要打开”。

#### Windows x64 无签名安装包

Windows 版本只在 GitHub Actions 的 `windows-2022` runner 上构建和测试。本地 macOS 开发环境不需要安装 Windows Rust target、Tauri CLI、NSIS 或其他 Windows 工具链，也没有 Windows 本地构建命令。

Release 中的直接下载文件为：

```text
Lumi-windows-x64-setup.exe
Lumi-windows-x64-setup.exe.sha256
```

可以在 PowerShell 中运行 `Get-FileHash .\Lumi-windows-x64-setup.exe -Algorithm SHA256`，将结果与 `.sha256` 文件比较。从 Actions 页面下载的 `Lumi-windows-x64` artifact 是包含上述两个文件的 ZIP 容器。

安装程序采用 NSIS 当前用户模式，不要求管理员权限，默认安装到 `%LOCALAPPDATA%\Lumi`。应用数据保存在独立的 `%LOCALAPPDATA%\dev.lumi.Lumi`，日志位于 `%LOCALAPPDATA%\dev.lumi.Lumi\logs\lumi.log`；release 包默认只记录 Warning/Error，达到 5 MiB 后轮转并保留 `.1`、`.2` 两个备份。卸载应用不会删除这些用户数据。早期预览包可能创建的 `%USERPROFILE%\.lumi` 不会被读取或自动迁移。Windows 安装程序、Tauri 启动器和 `lumi_web.exe` 均没有 Authenticode 签名，因此 Microsoft Defender SmartScreen 可能显示“Windows 已保护你的电脑”；仅应在确认下载来源和 SHA-256 后选择继续运行。

Windows workflow 不读取 Azure Secret，不安装或调用 `trusted-signing-cli`，也不配置 Authenticode 证书或时间戳服务。该产物仍是可验证的真正 unsigned 构建，不是对 Windows 签名检查的绕过。Tauri updater 使用独立的 Minisign 密钥验证更新包，不能替代 Authenticode，也不会消除 SmartScreen 提示。

### GitHub Actions 发布

[Desktop release workflow](.github/workflows/desktop-release.yml) 会在推送 `v*.*.*` Tag 时调用 [macOS build workflow](.github/workflows/desktop-macos.yml) 和 [Windows build workflow](.github/workflows/desktop-windows.yml)。两个平台并行构建、测试并上传 workflow artifact；只有两边全部成功，协调任务才会创建 Draft Release、生成统一的 `latest.json`、上传完整资产并发布。这样客户端不会读到只包含一个平台的更新清单。Windows workflow 仍会在 pull request 中构建和测试，但不会生成 updater artifact 或发布 Release。

三个 workflow 都可以从 Actions 页面手动运行。单独运行平台 workflow 只构建所选分支的预览 artifact；从 Desktop release workflow 勾选 `publish_release` 时，输入的 Tag 必须已经推送到 GitHub，它会检出 Tag 对应提交并执行完整发布，但不会代为创建 Tag。失败的 Draft 可以重跑并覆盖不完整资产；已经发布的 Release 视为不可变，必须创建新的 patch Tag，不能覆盖已被客户端引用的 updater 包或签名。

macOS workflow 会在干净的 runner 上预检桌面图标和脚本，再执行 Rust 测试、构建并启动 `.app`、校验 health response、系统浏览器调用、子进程退出与 codesign，最后生成：

```text
Lumi-macos-aarch64.app.zip
Lumi-macos-aarch64.app.zip.sha256
Lumi-macos-aarch64.app.tar.gz
Lumi-macos-aarch64.app.tar.gz.sig
```

Windows workflow 会在干净的 `windows-2022` runner 上安装固定版本的 Tauri CLI、从源 PNG 临时生成 ICO，执行 Go 和 Rust 测试、构建前端及 x64 Go 后端、生成 NSIS 安装程序，并完成 unsigned 状态、静默安装、health response、loopback 监听、日志和命令行令牌泄漏、进程清理及静默卸载检查，最后生成：

```text
Lumi-windows-x64-setup.exe
Lumi-windows-x64-setup.exe.sha256
Lumi-windows-x64-setup.exe.sig
```

稳定 Tag 必须严格使用 `vX.Y.Z`，发布后会成为 GitHub Latest Release，并额外包含带 `darwin-aarch64` 和 `windows-x86_64` 平台记录的 `latest.json`。`vX.Y.Z-beta.1`、`vX.Y.Z-rc.1` 等带后缀 Tag 会发布为 GitHub Prerelease，不进入 stable 自动更新通道。

Makefile 会先从 `origin` 同步 tag，并根据最新的稳定 SemVer tag 创建下一个本地 annotated tag。创建 tag 前工作区必须干净；首次发布、仓库还没有版本 tag 时从 `v0.1.0` 开始。

```bash
make tag-patch # v0.1.1 -> v0.1.2
make tag-minor # v0.1.2 -> v0.2.0
make tag-major # v0.2.0 -> v1.0.0
git push origin v0.1.2
```

也可以使用 `make tag TAG_BUMP=patch|minor|major`。命令只创建本地 tag，不会自动 push；推送后才会触发发布 workflow。

Tag 中去掉 `v` 的版本号会注入 Tauri bundle，因此不需要为了每次发布手工修改 `Cargo.toml`。

默认没有外部凭据也能完成 ad-hoc codesign 构建。若需要使用 Developer ID 证书，在 GitHub 仓库配置：

| 类型 | 名称 | 内容 |
|---|---|---|
| Actions variable | `APPLE_SIGNING_IDENTITY` | 证书完整 identity，例如 `Developer ID Application: ...`；不设置时为 `-` |
| Actions secret | `APPLE_CERTIFICATE_BASE64` | `.p12` 证书的 base64 内容 |
| Actions secret | `APPLE_CERTIFICATE_PASSWORD` | `.p12` 密码 |
| Actions secret | `TAURI_SIGNING_PRIVATE_KEY` | Tauri updater 私钥内容 |
| Actions secret | `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | updater 私钥密码 |

updater 密钥通过 `cargo tauri signer generate` 离线生成，公钥保存在 `rel/app/src-tauri/tauri.conf.json`，私钥和密码只保存在 GitHub Secrets，并必须另做安全的离线备份。私钥丢失后，已经安装的客户端无法信任使用新密钥签名的后续更新。workflow 不会打印这些 Secret。

接入 updater 之前安装的 v0.1.4 及更早版本没有内置 updater 公钥，必须手动安装首个支持自动更新的稳定版；从下一个 patch 版本开始才能验证完整的自动升级链路。当前 macOS 发布没有 Apple notarization，Windows 发布没有 Authenticode 签名，因此 ad-hoc 签名的 macOS 构建仍可能被 Gatekeeper 阻止，Windows unsigned 构建仍可能被 SmartScreen 提示。当前阶段不包含 macOS Intel、Universal Binary、DMG、Mac App Store、Windows ARM64、MSI 或 portable ZIP。

</details>
