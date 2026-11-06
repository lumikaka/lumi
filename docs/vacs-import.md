# 将 VACS 项目包转换为 Lumi 项目

## 使用

从 VACS 项目概览下载“VACS 项目包（用于 Lumi）”，保持 ZIP 文件完整，在本地运行：

```sh
convert-vacs-to-lumi "我的绘本.vacs.zip" --output "./我的绘本"
```

完成后，在 Lumi 中使用“打开已有项目”，选择输出的 `我的绘本` 文件夹。CLI 直接接收 ZIP，自动校验、解包、初始化并写入项目目录及 SQLite。输出目录必须尚不存在，包括空目录；不会覆盖已有项目、文件或链接。项目仅在全部内容完成、数据库关闭/checkpoint 并通过正常打开后发布；普通转换错误会清理临时内容。

运行只需要本地 CLI 和下载包。用户不需要连接 VACS、启动额外服务、配置模型或重新下载素材。VACS 线上构建和导出不依赖本工具；工具源码、构建及版本兼容由 Lumi 仓库负责。

## 本机安装

在 Lumi 仓库中，使用 `go.mod` 所要求的 Go 工具链安装：

```sh
make install-convert-vacs-to-lumi
convert-vacs-to-lumi --version
```

默认安装到 `~/.local/bin`，请确保该目录在 `PATH` 中。更新源码后重新运行同一命令即可更新工具。可通过 `GOBIN` 指定其他绝对路径：

```sh
make install-convert-vacs-to-lumi GOBIN="/自定义路径/bin"
```

## 构建或分发

在 Lumi 仓库中，使用 `go.mod` 所要求的 Go 工具链构建本机程序：

```sh
go build -trimpath -o ./convert-vacs-to-lumi ./cmd/convert-vacs-to-lumi
./convert-vacs-to-lumi --version
./convert-vacs-to-lumi "我的绘本.vacs.zip" --output "./我的绘本"
```

可以将构建后的可执行文件单独复制或分发给同平台、同架构的用户。运行二进制无需 Go 环境或 Lumi 源码。支持 macOS、Linux 和 Windows 的不覆盖发布操作；Windows 文件名为 `convert-vacs-to-lumi.exe`。跨平台构建时按目标设置 `GOOS` / `GOARCH`，例如：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ./convert-vacs-to-lumi-linux-amd64 ./cmd/convert-vacs-to-lumi
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o ./convert-vacs-to-lumi-windows-amd64.exe ./cmd/convert-vacs-to-lumi
```

发布时固定 Lumi commit 并附二进制 SHA-256。当前输入协议为 `vacs-project` v1。旧的原生 Lumi 项目 ZIP 已经是项目目录归档，不是本命令的输入，CLI 会明确拒绝。

## 格式和校验

ZIP 根必须有 `manifest.json`，其中 `format = "vacs-project"`、`version = 1`。清单包含名称、语言、故事总纲、有序章节/正文/页面/分镜、当前成图、全部有效设定原图和拆分素材、参考关系及提示词。

每个 `files[]` 条目含 `id/path/filename/size/sha256`，可选 `sha1/locations/register_purpose`；`path` 必须是 `files/` 下安全相对路径。每个 `prompts[]` 条目含 `group/key/value/path/size/sha256`，路径为 `prompts/` 下 `.txt` 副本。`attachments[]` 登记 `README.md` 和 `image-prompts.json` 的路径、大小与 SHA-256。CLI 在创建项目之前验证全部登记文件大小和摘要、提示词副本与 value 一致、ID 及关系完整，拒绝不支持版本、缺失/损坏文件、不安全或重复路径、链接、文件与目录冲突及未登记的额外文件。错误指出对应文件 ID、路径或章节/页面位置。

资源限制为最多 100,000 个 ZIP 条目、64 MiB manifest、单条目 2 GiB、总展开 20 GiB。现有 Lumi 字段与素材格式校验也会执行；错误不会发布项目。VACS 的合法长标题由 CLI 适配：项目显示名最多 120 个 Unicode 字符，页面显示标题最多 160 个；超出时保留前若干字符和省略号，恰好达到上限时保持原文。完整原名始终保留在 `user-data/vacs/manifest.json`，每次调整的来源位置、上限、原文和显示值写入 `conversion-notices.json`，CLI 成功输出同时列出 `project/<原项目ID>` 或 `chapter/<原章节ID>/page/<原页面ID>` 提示。

## 恢复内容

`internal/vacsimport` 复用 Lumi 项目初始化、数据库迁移、Story / Production / Asset Store 服务，无独立 SQLite schema。恢复条漫规格、故事总纲、全部有效章节及页面顺序、正文、分镜、当前图像和共享参考素材。素材库包含未被页面引用的原图与拆分素材；拆分素材当前版本的来源指向相同的原始设定图，保留归一化 `crop_box`。`content-mapping.json` 对照原始 ID 和 Lumi UUID。不同设定即使共用相同源文件，也分别登记、按清单中的设定 ID 关联；页面不会因图片字节相同而被改到另一设定。VACS 的旧引用歧义及原始引用保留在 `manifest.json` 的页面 `reference_warnings` 和合成素材 `original_reference`，供人工确认。

当前画稿的实际提示词、修订提示词、哈希及参考图写入导入图片快照，由正常图片详情读取，不创建虚假的 Lumi 生成任务。所有原文件和候选图保留于 `user-data/vacs/files/`；`manifest.json`、`image-prompts.json`、`prompts/*.txt` 保存来源、归属和全部有效提示词（包含默认值）。`prompts/mapping.json` 记录映射结果。首版只映射用途兼容且无模板变量的整体风格；其他任务模板及候选版本作为附件保留。历史只有哈希时明确记录原文缺失，不用当前模板倒推。聊天与运行/计费历史跳过。

## 测试与两仓库验收

默认测试自行创建全部样本，不依赖 VACS 仓库或临时复现目录。包含共用同一文件的两个设定选择第二个后正常打开的身份断言，以及通过独立进程执行 CLI、120/121 字项目名、160/161 字页面标题、255 字标题和扩展区汉字的转换、提示输出、编辑保存、关闭重开及原文留档校验。另含小样本 ZIP → 目录 → 正常打开 → 图片/参考读取 → 编辑保存 → 关闭重开，以及错误版本、损坏文件、路径攻击、既有输出和发布竞争保护：

```sh
go test ./internal/vacsimport ./cmd/convert-vacs-to-lumi ./internal/project ./internal/story ./internal/production ./internal/files
```

VACS 的 `project_package_controller_test.exs` 默认验证完全无转换器时走正常导出 API，并支持将真实下载包写入 `VACS_PACKAGE_SAMPLE_ZIP`、`VACS_PACKAGE_TEXT_ZIP`、`VACS_PACKAGE_UPDATED_ZIP`、`VACS_PACKAGE_SETTINGS_ZIP`、`VACS_PACKAGE_REVIEW_ZIP` 指定位置。将这些 ZIP 和本机 CLI 复制到独立目录，分别运行上述用户命令，得到原生项目目录。再指向这些 CLI 产物运行：

```sh
VACS_CONVERTED_SAMPLE=/tmp/完整样本 VACS_CONVERTED_TEXT=/tmp/纯文字 VACS_CONVERTED_UPDATED=/tmp/更新提示词 VACS_CONVERTED_SETTINGS=/tmp/只有设定 VACS_CONVERTED_REVIEW=/tmp/四项修复样本 go test -v ./internal/vacsimport
```

验收复制 CLI 产物后通过正常项目打开和领域服务验证内容、文件原始字节、共用设定、未引用原图及其裁切素材、提示词更新，并编辑缺图页后关闭重开；原始 CLI 产物保留，测试可重复运行。用户下载后无需修补数据库或手工导入任何文件。
