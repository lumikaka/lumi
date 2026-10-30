# Step 0 · Codex 关键行为调研

验证日期：2026-10-30（Asia/Shanghai）。本步只新增本文、`spike/` 内的一次性脚本与证据，未修改 Lumi 产品代码或用户已有 Codex 配置。

**结论：stdio 按会话 cwd 绑定项目、宿主生图路径导入、参考图编辑、本地 marketplace 安装均有实测依据。项目写保护在 macOS CLI 沙箱中有效，但依赖项目信任和所选权限模式。MCP Apps 的后续 [Step 4.5 验收](mcp-apps-acceptance.md) 已验证真实渲染、桥接和工具取图，支持 Step 5 采用 App 内只读工具返回受控缩略图；loopback allow 轮失败，CSP 对照及写操作批准/拒绝仍未验证。**

## 环境与证据边界

| 项目 | 实测值 |
| --- | --- |
| 系统 | macOS 14.5 / 23F79，arm64 |
| PATH CLI | `/Users/qingyang/.local/bin/codex`，`codex-cli 0.163.0` |
| App | `/Applications/ChatGPT.app`，版本 `26.1027.30824`，build `22943`，bundle ID `com.openai.codex` |
| App 内置 CLI | `/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex`，`codex-cli 0.166.0-alpha.5.1` |
| stdio 测试 | 两个版本的真实 `codex app-server`，同一 server 中启动不同 cwd 的两个 thread；无模型推理、无登录凭据 |
| 生图 / Subagent 测试 | 本次真实 App 对话；附带的 CLI 版本如上，不把 PATH CLI 版本当作 App 的版本 |
| 配置 / Hooks / 沙箱 | PATH CLI `0.163.0` 与 App 内置 CLI `0.166.0-alpha.5.1`，临时 git 项目、临时 `CODEX_HOME`，数据库为普通文本假文件；App 另有用户人工反馈 |

App 的版本来自已安装 bundle，未另行核验运行中服务端的二进制指纹。所有版本结论仅覆盖以上环境；不是未来版本或跨平台保证。

证据索引及运行说明见 [`spike/README.md`](spike/README.md)。`evidence/` 保存原始 RPC 结果、MCP JSONL、hook 输入、实际 PNG 和元数据。证据里的 `/private/tmp/...` 是本次运行位置，**复现时须使用脚本新打印的目录**。脚本不复制用户认证信息，也不读取真实 Lumi 数据库。

## 1. stdio MCP 的 cwd 与进程范围

**结论：已验证。** 在插件 `.mcp.json` 不指定 `cwd` 时，stdio 进程的 `os.getcwd()` 是该 thread 的工作目录，而非插件缓存目录或 app-server 启动目录。两个版本一致。

| 版本 | app-server 启动目录 | 会话 A：cwd / 工具 PID | 会话 B：cwd / 工具 PID |
| --- | --- | --- | --- |
| `0.163.0` | `.../lumi-codex-spike-cq3f4ycu/launcher` | `.../project-a` / `98557` | `.../project-b` / `98635` |
| `0.166.0-alpha.5.1` | `.../lumi-codex-spike-ocw4qvr1/launcher` | `.../project-a` / `98433` | `.../project-b` / `98545` |

同一 thread 连续两次 `inspect` 返回相同 PID。不同 cwd 的 thread 使用不同工具进程。**不能据此承诺“每个会话恰好一个 MCP 进程”**：每个 cwd 还观测到另一条初始化、列工具和列资源的连接，具有不同 PID；没有证据确定额外连接的完整生命周期。

初始化参数实录要点：

```json
{
  "protocolVersion": "2025-06-18",
  "capabilities": {
    "experimental": { "codex/auth-change": {} },
    "elicitation": { "form": {}, "url": {} }
  },
  "clientInfo": { "name": "codex-mcp-client", "title": "Codex", "version": "0.163.0" }
}
```

没有 cwd / workspace 参数。客户端没有声明 roots 能力；探针仍发送 `roots/list`，收到 `{"roots":[]}`，因此不能依赖 roots 找项目。本次检查的 `PWD`、`CODEX_HOME`、`CODEX_THREAD_ID`、`CODEX_SESSION_ID`、`CODEX_CWD`、`CODEX_WORKSPACE_ROOT`、`PLUGIN_ROOT` / `PLUGIN_DATA` 及 CLAUDE 兼容变量均未出现在 MCP 环境中。这里只说明这些变量的观测值，不声称穷尽所有潜在变量。显式声明的 `SPIKE_LOG` 正常传入。工具调用 `_meta.threadId` 可见，但不是项目路径。

**复现：**

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --codex /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex
# 查看每次打印的 ROOT/report.json 与 ROOT/mcp.jsonl
```

生成器创建一个本地 marketplace，插件通过 `.mcp.json` 的绝对 Python 路径启动 `probe_server.py`；启动时不设置 `cwd`。随后依次执行 `thread/start(cwd=A/B)`、`mcpServerStatus/list(threadId)` 和两次 `mcpServer/tool/call(inspect)`。

**不确定 / 未验证：** GUI 当前会话直接安装该探针后的启动行为、TUI/IDE 前端的差异、重连/卸载后的回收、同一个 thread 中途切换 cwd、显式 MCP `cwd` 覆盖、Subagent 自己的 MCP 连接复用。Lumi 应在 `lumi mcp` 启动时固定并校验 cwd；不要跟随模型每次提供的任意项目路径，也不要以 MCP PID 充当 Lumi 实例的唯一性依据。

证据：[`stdio-image.json`](spike/evidence/stdio-image.json)、[`stdio.jsonl`](spike/evidence/stdio.jsonl)、[`app-bundled-stdio-image.json`](spike/evidence/app-bundled-stdio-image.json)、[`app-bundled-stdio.jsonl`](spike/evidence/app-bundled-stdio.jsonl)。

## 2. 原生生图产物与路径传递

**结论：App 生图、实际落盘与 MCP 读取已验证；CLI 内由模型发起原生生图未验证。** 版本：App `26.1027.30824` / 内置 CLI `0.166.0-alpha.5.1`；读取路径另以 CLI `0.163.0` 和内置 CLI 复测。

本次内置 `image_gen.imagegen` 返回的是 `image_url` 和 `output_hint`，没有直接返回名为 `savedPath` 的字段。提示中给出实际路径：

```text
/Users/qingyang/.codex/generated_images/
  01a18be4-9a79-7941-b214-3748669c12ae/
  exec-65413b3a-e8d3-4b30-bf28-0b082ec1f18e.png
```

上面换行仅为展示，真实路径为一行。CLI `0.163.0` 导出的 app-server 类型另有可选 `ImageGenerationItem.savedPath`，但本次没有通过那个事件接口生成图片，不能把类型声明当作实际事件记录。导出类型见 [`ImageGenerationItem.ts`](spike/evidence/ImageGenerationItem.ts)。

| 控制项 | 实测 |
| --- | --- |
| 文件名 / 目录 | 工具自行生成 `exec-<UUID>.png`；本次工具参数没有输出路径或文件名字段。导入后再由 Asset Store 命名 |
| 格式 | 三次均为真实 PNG；本次没有独立 `format` 参数。JPEG/WebP 原生输出未验证 |
| 尺寸 | 提示词请求 `384×256`，原图及狐狸编辑图实际均为 `1536×1024`；方形参考编辑实际 `1254×1254`。宽高比能被提示引导，精确像素没有兑现 |
| 透明背景 | 工具有 `transparent_background` 参数，本次为 false；透明输出未验证 |
| 模型 / 编码质量 | 当前工具没有暴露对应参数；具体图像模型未从结果中确认 |

第一张图为 `1,889,711` 字节，SHA256 为 `f54be7a2cc0671df5716c36dc7c37afa5601c91191b159ad1b0b99f4947cf040`。模型在本对话中取得提示中的路径，构造 `read_image({path})` 参数，再经真实 app-server 调用已安装插件。两个版本、两个 cwd 的 MCP 进程均成功读取原始路径并返回相同字节数和摘要；无需先复制到项目目录。MCP 是独立传输/进程边界，不能把本地命令沙箱等同于 MCP 权限；官方说明见 [Permissions](https://learn.chatgpt.com/docs/permissions#scope-and-enforcement)。

**复现：**

1. 在 App 中要求内置生图生成一张带 `384×256 PNG` 尺寸要求的测试图。完整原始工具参数保存在 [`image-generation.json`](spike/evidence/image-generation.json)。
2. 从工具返回中取得本次真实路径，替换下列路径：

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --image '/absolute/path/from-the-image-tool.png'
# ROOT/report.json → rpc.project-a.image_read / project-b.image_read
```

**不确定 / 未验证：** 这里的工具参数由本对话模型编排，经 app-server RPC 发给插件；未验证一个独立 `codex exec` 模型轮次自主选择该 MCP 工具的端到端过程。CLI 原生生图的事件形状、远程宿主/容器是否共享文件系统、不同 OS 路径、产物保留期限及清理策略也未验证。Step 4 不应扫描固定文件名或硬编码 `savedPath` 字段；应让模型传入宿主实际提供的路径，并立即校验、导入。

证据：上述两个版本的 `stdio-image` 报告、[`image-generation.json`](spike/evidence/image-generation.json)、[`fox-original.png`](spike/evidence/fox-original.png)。

## 3. 生图与对话参考图

**结论：App 对话中的已有图片可以作为参考，本次两种来源均实测成功。** 版本：App `26.1027.30824`，内置 CLI `0.166.0-alpha.5.1`；MCP 来源图由 PATH CLI `0.163.0` 的真实 stdio server 返回。

| 参考来源 | 操作与实测效果 |
| --- | --- |
| 上一次宿主生图 | 使用 `num_last_images_to_include: 1`，要求同一薄荷绿狐狸只增加四只红雨靴。保留了白色耳朵星斑、紫色围巾、橙色挎包、面部、背景与大体姿态；四脚均出现红靴。纹理有细微变化，并非逐像素不变 |
| MCP `content.type=image` | `reference_image` 返回 `128×128` PNG：左红右蓝、左上黄块。将这个真实 MCP 结果作为 image content 转呈本对话后，只把最近一图传给生图，要求增加居中黑色空心圆环。结果保留左右分区、黄块位置和相对大小，新增黑环；颜色出现轻微渐变，尺寸变为 `1254×1254` |

第二项的路径是 **真实 stdio MCP → app-server RPC → 本对话 `functions.image` 转呈 → 内置生图**。没有把参考图本地路径传给生图。它验证了进入对话的 MCP 图片内容可被引用；尚未验证“将探针直接安装到当前 App 会话并直接返回图片”的同前端路径，不混为一项证据。

**复现：** 先运行第 1 项生成临时探针，再让模型执行：

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/call_probe.py \
  '/private/tmp/lumi-codex-spike-REPLACE_WITH_NEW_ROOT' reference_image '{}'
```

将结果 `result.content` 中的 image 块通过宿主图像展示接口送入当前对话，再调用内置生图：

```json
{
  "num_last_images_to_include": 1,
  "transparent_background": false,
  "prompt": "Use the immediately preceding MCP image as reference. Preserve red left half, blue right half, yellow top-left square. Add one thick black hollow ring centered in the square."
}
```

须确保最近一张图就是目标参考。原始工具参数与完整英文提示词见 `image-generation.json`。

**不确定 / 未验证：** 多张参考、长对话跨压缩后引用、真实复杂角色从 MCP 返回后的保真度、批量生成和稳定角色一致性。本次是可行性样本，没有给一致性成功率。CLI / IDE 的同等参考链路未验证。

证据：[`fox-original.png`](spike/evidence/fox-original.png)、[`fox-boots.png`](spike/evidence/fox-boots.png)、[`mcp-reference-result.json`](spike/evidence/mcp-reference-result.json)、[`mcp-reference.png`](spike/evidence/mcp-reference.png)、[`mcp-reference-ring.png`](spike/evidence/mcp-reference-ring.png)。

## 4. MCP Apps：界面、CSP、图片与回调

**后续验收更新（2026-10-30）：** [Step 4.5](mcp-apps-acceptance.md) 已验证真实渲染/初始化、本地六档 data URI、两种工具取图模式 × 六档及只读/临时写入/错误回显；8 MiB padding 仅为已测下界。allow 轮 loopback 失败，no-resource 对照和写操作批准/拒绝仍待验证。以下保留早期调研快照与原始复现方法，其中“未验证”描述当时状态；当前图片方案以 Step 4.5 和 [Step 5](step5-mcp-apps.md) 为准。

**结论：资源关联、列出/读取和 HTML/CSP 传输已验证；实际 iframe 行为未验证。** 版本：CLI `0.163.0`、App 内置 CLI `0.166.0-alpha.5.1`；观察到的 App 版本 `26.1027.30824`。

探针的 `show_app` 工具声明 `_meta.ui.resourceUri = "ui://lumi-spike/app.html"`；资源返回 `text/html;profile=mcp-app`。两个版本均能发现工具并通过 `mcpServer/resource/read` 取得带 CSP 的 HTML。这只证明 MCP 传输，不证明画面已渲染。

| 界面 | 本次证据与结论 |
| --- | --- |
| App | UI inventory 确认当前会话存在 `Codex MCP Apps` 浏览器通道（type `mcpapps`），当时 tabs 为空。官方提供 MCP Apps UI 桥接说明；**本探针在真实 iframe 中的渲染未验证** |
| IDE | 官方 [Plugins](https://developers.openai.com/codex/plugins#plugin-directory-in-the-ide-extension) 说明 IDE 不支持插件。这不等于证明所有直接配置 MCP 的 UI 行为；IDE 的 MCP Apps 渲染**未验证**，本次未启动 IDE 验收 |
| TUI | 真实 CLI 的插件/纯数据 MCP 路径可用；`features list` 中 `enable_mcp_apps` 为 under development / false。**TUI 的 MCP Apps 渲染未验证**，没有把 feature 名当作可用界面的保证 |

官方 UI 标准使用 `_meta.ui.csp`，静态图片 origin 放在 `resourceDomains`；JS 请求对应 `connectDomains`。旧的 `openai/widgetCSP` 是 snake_case 的兼容元数据。见 [Add UI](https://developers.openai.com/plugins/build/chatgpt-ui) 和 [Plugin reference](https://developers.openai.com/plugins/reference)。本探针候选声明为：

```json
{
  "_meta": {
    "ui": {
      "csp": {
        "resourceDomains": ["http://127.0.0.1:18765"],
        "connectDomains": ["http://127.0.0.1:18765"]
      }
    }
  }
}
```

**这不是已验证的 loopback 许可。** `resourceDomains` 是声明方式；是否允许 HTTP loopback、是否受宿主额外网络限制，不能仅凭 CSP 推断。`connectDomains` 也不能替代图片的 `resourceDomains`。

- `http://127.0.0.1:<port>/reference.png` 在 App iframe 内是否加载：**未验证**。
- data URI 能否显示及最大体积：**未验证，没有可报告的上限或已测下界**。不能把“没查到限制”写成无限制，也不能先声称 loopback 失败再选择 data URI。
- App 内回调工具：标准支持 `tools/call`，探针按钮按该协议调用 `inspect`，并显式声明工具 `visibility: ["model", "app"]`；**实际点击/回调、批准弹窗及状态更新未验证**。

原因：不允许修改用户现有 `~/.codex` 配置；临时 `CODEX_HOME` 的 app-server 没有 GUI renderer；当前 App 会话没有把临时 server 注册成 MCP App 的工具入口。没有用普通浏览器加载 HTML 来冒充宿主 iframe 验证。

**可复现的下一步人工验收：**

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py
# 使用本次 ROOT；脚本已生成 ROOT/reference.png
python3 -m http.server 18765 --bind 127.0.0.1 --directory '/private/tmp/NEW_ROOT'
```

在另行准备的、允许安装测试插件的 App 环境中安装 `ROOT/marketplace`，新建会话调用 `show_app`：

1. 记录 App 及其内置 `codex --version`；截图确认 UI 真正出现。
2. 确认 loopback 图片为 loaded，并检查 server 的 HTTP 请求日志。对比去掉 `resourceDomains` 的版本，每次更新插件/会话，避免缓存干扰。
3. 测试 data URI padding 为 0、64 KiB、256 KiB、1 MiB、4 MiB、8 MiB。探针插入有效 PNG `tEXt` chunk 并显示最终 URI 字符数；某值成功只能作为该版本下界。它测试 iframe 内图片解码，**不测试大型 data URI 经 MCP 工具结果传输时的 payload 上限**；后者须另测。
4. 点击 Call inspect，确认 server 收到新的 `tools/call` 且界面显示结果。另测写工具的批准与错误反馈，不以只读按钮替代。

早期调研时该清单尚未执行；后续真实宿主验收结果及剩余未验证项见 [Step 4.5](mcp-apps-acceptance.md)。Step 5 保留完整文本兜底。

证据：上述两个版本 `stdio-image` 报告的 `mcp_status` / `show_app` / `ui_resource`，以及 [`app-inventory.json`](spike/evidence/app-inventory.json)。

## 5. 沙箱写保护与项目配置的信任

**结论：已验证 macOS PATH CLI 与 App 内置 CLI 下可以用项目级 permission profile 限制写入；当前 App 中另获用户人工确认样本；不是不可覆盖的强制策略。** 版本：PATH `codex-cli 0.163.0`，App bundle `0.166.0-alpha.5.1`。Windows / Linux 的同等规则执行**未验证**。

临时项目 `.codex/config.toml`：

```toml
default_permissions = "lumi-protect"

[permissions.lumi-protect]
extends = ":workspace"

[permissions.lumi-protect.filesystem.":workspace_roots"]
"project.sqlite" = "read"
"project.sqlite-wal" = "read"
"project.sqlite-shm" = "read"
"project.sqlite-journal" = "read"
".lumi" = "read"
"assets" = "read"
```

项目标记为 untrusted 时，`config/read(includeLayers=true)` 返回该 project layer 的 `disabledReason`，有效 `default_permissions` 为空。改为 trusted 后，该配置生效，`thread/start` 的 `activePermissionProfile` 为 `{"id":"lumi-protect","extends":":workspace"}`。因此**不能只生成配置文件就宣称保护已开启**。

在真实 Codex Seatbelt 沙箱中实测：

| 操作 | 结果 |
| --- | --- |
| 写 `allowed.txt` | 成功 |
| 覆盖假 `project.sqlite`、WAL、SHM | 全部 EPERM |
| 创建原先不存在的 `project.sqlite-journal` | EPERM |
| 覆盖 `.lumi/probe.txt`、`assets/probe.txt` | 全部 EPERM |
| 新建 `assets/new.txt` | EPERM |
| 用普通临时文件原子替换数据库、删除数据库、重命名 `assets/` | 全部 EPERM |

**复现：**

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py
# 执行脚本打印的 sandbox_command（目录由本次输出替换）
CODEX_HOME=/private/tmp/NEW_ROOT/home codex sandbox \
  -P lumi-protect -C /private/tmp/NEW_ROOT/project \
  -- python3 /absolute/path/to/lumi/docs/features/codex-plugins/spike/write_probe.py
```

`codex sandbox` 在本版本需要显式 `-P`；缺失会报 required argument。外层 Codex 沙箱不允许嵌套 `sandbox-exec`，所以本次测试命令获自动审批后在外层沙箱之外启动，再由被测 Codex 加载自己的 Seatbelt 规则；所有写操作都只针对 `/private/tmp/` 的假文件。

Step 2 用 App 内置二进制复测：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py \
  --codex /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex
# 再执行本次打印的 sandbox_command；不要复用历史夹具路径
```

本次夹具为 `/private/tmp/lumi-codex-config-xbgwj5qx/project`。untrusted layer 带 `disabledReason`，thread 的 active profile 为 `:workspace`；trusted 后为 `lumi-protect`。上表 11 项操作结果与 PATH CLI 一致，受保护项全部返回 EPERM，只有 `allowed.txt` 成功。

随后请用户在 App 打开同一临时假项目、手动信任并新建默认权限会话，不批准提权。用户人工反馈：“已向 allowed.txt 追加 Codex write test”，并报告 `project.sqlite` 与 `assets/` 受当前环境只读限制、未写入。此确认来自用户转述的 agent 反馈，未另收集 App 工具执行日志、有效权限配置或 GUI 进程指纹；可记录为当前 App 样本的人工确认，不能扩展为所有 App 场景均受保护。附属文件及替换/删除/重命名的逐项证据来自内置 CLI 测试。

**限制与设计影响：**

- 用户必须信任项目；不要自动把用户实际项目写成 trusted。本次只在临时 `CODEX_HOME` 设置测试信任。
- CLI/session 的更高优先级设置可改变所选策略。官方说明旧 `sandbox_mode` / `--sandbox` 与新 profile 不组合，旧设置可能使 `default_permissions` 不生效，详见 [Permissions](https://learn.chatgpt.com/docs/permissions) 与 [Configuration precedence](https://developers.openai.com/codex/config-file/config-basic#configuration-precedence)。不能把该片段描述为任何模式下都有效。
- 本次验证了文件操作，未测模型的 `apply_patch`、所有替换手法或批准后的提权。`AGENTS.md` 是行为指引，不代替 OS 限制。
- `read` 仍允许读取；若 Step 2 要禁止直接读，也需要另设计/验证 deny-read。该片段只兑现本题的写保护。
- MCP、本地 Lumi server 与用户批准的其他执行路径有各自权限边界；不要期待这份命令沙箱配置限制 Lumi 的正常资产写入。数据库锁、版本校验仍须由 Lumi 实现。

证据：[`config-hooks.json`](spike/evidence/config-hooks.json)、[`sandbox-writes.json`](spike/evidence/sandbox-writes.json)、[`app-bundled-config-hooks.json`](spike/evidence/app-bundled-config-hooks.json)、[`app-bundled-sandbox-writes.json`](spike/evidence/app-bundled-sandbox-writes.json)、[`app-manual-write-confirmation.json`](spike/evidence/app-manual-write-confirmation.json)。

## 6. Subagent：触发、上下文与插件工具

**结论：真实 App 内已验证显式触发、两种上下文继承选项与插件 MCP 调用；Subagent 不是文件系统隔离。** 版本：App `26.1027.30824` / bundle CLI `0.166.0-alpha.5.1`；PATH `codex --version` 为 `0.163.0`。独立 CLI Subagent 模型轮次未验证。

当前触发方式是用户明确要求，或适用的项目/skill 指令明确要求。不能让 Step 6 的 skill 依赖模型在任何任务中都会自动开 Subagent。官方说明及自定义 agent 配置见 [Subagents](https://developers.openai.com/codex/multi-agent)。本次以用户第 6 项验证需求启动两个测试子代理：

| 测试 | 实测 |
| --- | --- |
| `fork_turns="none"` | `/root/subagent_probe` 未收到父对话历史；仍有系统/开发者指令、AGENTS/environment 和委派任务。cwd 与父相同；读到父创建的 `probe_server.py` |
| `fork_turns="all"` | `/root/subagent_fork_probe` 能从父历史复述原始 8 项需求、两种准确 CLI 版本与第一张图实际尺寸；任务本身没有提供这些数值 |
| 插件 MCP | none 子代理通过 `ALL_TOOLS` 发现并调用现有 `mcp__plan_questionnaire__get_questionnaire`；服务端对虚构 ID 返回 `{"isError":true,"content":[{"type":"text","text":"找不到该问卷，请重新打开问卷"}]}` |

最后一项证明子代理能发现并实际派发插件 MCP 工具；它不是业务读取成功。没有创建/提交问卷或改动插件状态。

**复现（在支持相同工具的 App 对话中）：**

```text
请只做 Subagent 验证。以 fork_turns=none 启动一个子代理：
报告是否收到父历史和 cwd；读取指定的临时探针文件；
调用现有只读插件工具 get_questionnaire，使用不存在的测试 ID；
禁止创建/提交问卷、修改文件。返回完整服务端结果。
再以 fork_turns=all 启动另一个子代理，让它复述父对话已有、委派提示未给出的测试数值。
```

CLI 可用 `/subagents` 查看；这是官方指令，本次未打开该 TUI 面板验收。官方说明自定义代理继承父权限、provider 和 MCP servers；可通过配置缩减部分能力。不要把 none 理解为无规则、独立 workspace 或独立权限。

**未验证：** 新 Lumi stdio 插件在子代理中的具体 PID/cwd、长历史截断规则、后台批准交互、不同模型/自定义 agent 的工具集合差异。Step 3 可继续采用 `render_prompt → 子代理输出 → 有版本保护的写回`；用明确任务输入、公开 UUID 和 revision 约束，不能靠上下文隔离解决并发写入。

证据：[`subagents.json`](spike/evidence/subagents.json)（本次工具调用与子代理报告记录）。

## 7. Hooks：事件、transcript 与信任

**结论：12 种事件被当前 CLI 接受并列出；SessionStart 与 UserPromptSubmit 的真实输入含可读 transcript 路径。项目信任和 hook 信任是两道独立门槛。** 版本：`codex-cli 0.163.0`；本项未在 App 内置 `0.166.0-alpha.5.1` 复跑。

当前识别的事件：`SessionStart`、`SessionEnd`、`UserPromptSubmit`、`PreToolUse`、`PermissionRequest`、`PostToolUse`、`PreCompact`、`PostCompact`、`SubagentStart`、`SubagentStop`、`Stop`、`Interrupt`。真实 `hooks/list` 列出了全部 12 个配置项，且与 CLI 导出的 [`HookEventName.ts`](spike/evidence/HookEventName.ts) 一致。**列出不等于已逐一触发。**

项目 untrusted 时：项目 hooks 列表为空。项目 trusted 时：12 项显示 `enabled: true`、`isManaged: false`、`trustStatus: "untrusted"` 和各自的 `currentHash`；不会执行。官方要求 `/hooks` 审阅并信任准确的 hook 定义，变更后重新审阅，见 [Hooks](https://learn.chatgpt.com/docs/hooks#review-and-trust-hooks)。安装/启用插件也不自动信任其 hook。

为观察 payload，本次仅对脚本生成且已检查的无害测试 hook 使用单次 `--dangerously-bypass-hook-trust`；没有修改持久化信任记录。模型 provider 指向不可用的本地测试端口，轮次在 5 秒后终止，没有成功模型响应。由真实 CLI 发出的两个事件如下（路径缩写）：

```json
{
  "session_id": "01a18bf2-774d-70b2-ae2d-d90b4c660a1f",
  "transcript_path": "/private/tmp/.../home/sessions/2026/10/30/rollout-2026-10-30T16-34-45-01a18bf2-774d-70b2-ae2d-d90b4c660a1f.jsonl",
  "cwd": "/private/tmp/.../project",
  "hook_event_name": "SessionStart",
  "model": "gpt-6.1-sol",
  "permission_mode": "bypassPermissions",
  "source": "startup"
}
```

`UserPromptSubmit` 另含 `turn_id` 和 `prompt`。这里 `permission_mode` 原样记录为测试期间 CLI 报告值，不以它推断 OS 沙箱状态。hook 进程 cwd 等于项目目录，两次事件读取时 transcript 文件均已存在。随后检查文件为 JSONL，包含 `session_meta`、`event_msg`、`response_item`、`turn_context`、`world_state`；元数据摘要见 [`hook-transcript-metadata.json`](spike/evidence/hook-transcript-metadata.json)。没有保存用户实际聊天或复制完整 rollout。

**复现：**

```sh
codex --version
# 只测信任/发现，不执行 hook：
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py
# 检查脚本生成的 hook 后，显式选择单次执行阶段：
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py \
  --run-reviewed-hooks
# 查看本次 ROOT/report.json 的 hook_payloads 和 ROOT/hooks.jsonl
```

**未验证 / 限制：** `/hooks` 的正常人工信任流程未操作；插件打包 hook 未执行；Stop、PostToolUse、SessionEnd、SubagentStart/Stop、压缩、Interrupt 的实际触发与各自 payload 未验证。当前断网失败轮次不能证明成功轮次结束时能导入完整助手输出。官方标明 transcript 格式不稳定，路径也允许为空；不能把文件扩展名视为永久兼容承诺。Step 7 应默认关闭、容错、保留手动导入，先补 Stop/中断/恢复验收，不把 bypass 参数交付给普通用户作为安装默认值。

证据：[`config-hooks.json`](spike/evidence/config-hooks.json)、[`hooks.jsonl`](spike/evidence/hooks.jsonl)。

## 8. 本地 marketplace 插件安装与批准

**结论：从空临时 CODEX_HOME 安装本地 marketplace 插件已成功，两个 CLI 版本均可；临时目录内的安装没有出现额外 CLI 确认。** 版本：`0.163.0`、`0.166.0-alpha.5.1`。真实用户配置的安装批准体验**未验证**，因为本步禁止改动该配置。

以下为从零创建最小插件并安装的完整入口，无需网络、API key 或已有登录：

```sh
cd /absolute/path/to/lumi
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py
```

生成器先 `mkdtemp` 创建 ROOT/home、marketplace、launcher 及测试项目，然后写入：

```text
ROOT/marketplace/.agents/plugins/marketplace.json
ROOT/marketplace/plugins/lumi-spike/.codex-plugin/plugin.json
ROOT/marketplace/plugins/lumi-spike/.mcp.json
ROOT/marketplace/plugins/lumi-spike/probe_server.py
ROOT/marketplace/plugins/lumi-spike/app.html
```

其中 marketplace 的 `source.path` 为 `./plugins/lumi-spike`，相对 marketplace 根目录；`.mcp.json` 使用解释器与脚本的绝对路径。然后执行的确切 CLI 序列是：

```sh
# ROOT 必须取生成器本次输出；不是用户现有 ~/.codex
export CODEX_HOME='/private/tmp/lumi-codex-spike-REPLACE/home'
codex plugin marketplace add '/private/tmp/lumi-codex-spike-REPLACE/marketplace' --json
codex plugin add lumi-spike@lumi-spike-local --json
codex plugin list --json
# 生成器随后启动新 app-server，并在新 thread 中调用 inspect 验证可用
```

安装结果显示 `pluginId: lumi-spike@lumi-spike-local`、`enabled: true`，缓存位于 `CODEX_HOME/plugins/cache/lumi-spike-local/lumi-spike/0.1.0`。仅临时 `config.toml` 增加 `[marketplaces.lumi-spike-local]` 和 `[plugins."lumi-spike@lumi-spike-local"] enabled=true`；此案例不要求 OAuth。`plugin list` 不是最终验证，后续新 thread 的真实 MCP 调用也成功了。安装后新建会话符合官方 [CLI 插件使用说明](https://developers.openai.com/codex/plugins#plugin-directory-in-codex-cli)。

在 Codex 对话里让模型安装时，应区分以下批准边界：

| 动作 | 本次结果 / 后续需要 |
| --- | --- |
| 写入本次临时 CODEX_HOME、添加本地 marketplace、安装该探针 | 当前 workspace-write 环境允许，无额外 CLI 确认；不代表任意用户配置都免批准 |
| 写用户真实 `~/.codex`、下载/安装用户目录中的 Lumi 二进制 | 本步未执行。若超出宿主可写范围或网络权限，需要宿主相应批准/自动审查；具体 UI 与次数依模式、执行规则和组织策略而定 |
| 信任实际项目 | 项目级配置要生效须用户信任；本次只构造临时 trusted/untrusted 夹具，未替用户信任真实项目 |
| 启用 hook | 另需审阅/信任 hook 定义；插件安装成功不是 hook 执行许可 |
| 修改真实用户配置的最终“一句话安装”流程 | **未验证**；不得承诺一句话零批准、当前会话立即热加载或跨平台无差异 |

标准插件结构与 marketplace 路径说明见 [Build plugins](https://developers.openai.com/plugins/build/plugins)。这份 spike 的安装不下载 Lumi，也没有验证更新、幂等升级、卸载、签名、公证和 Windows；这些仍属于 Step 6。

## 对 Step 1–7 的影响

对应现有 `step1-*.md` 至 `step7-*.md`，本步不直接改写这些任务文件。

| 后续步骤 | 成立的假设 | 必须调整或保留的条件 |
| --- | --- | --- |
| Step 1 · CLI / server | 适合由独立 Lumi 实例统一管理项目 | 一个会话可能有多条 MCP 连接，多个会话有不同进程。不要把 server 生命周期或项目锁绑定某一 MCP PID；`serve` 的单实例与启动竞争保护仍必要 |
| Step 2 · init | 可以生成 macOS 上有效的项目 permission profile | 必须告知项目信任、识别 legacy sandbox 设置冲突并校验实际 profile；保护 SQLite sidecar。已有 `.codex/config.toml` 要保留/合并，不能覆盖；只读配置仍允许读取，跨平台尚待验证 |
| Step 3 · stdio / 文本 | 未指定 cwd 的插件进程可按会话 cwd 绑定项目；子代理可用插件工具 | 用 cwd 固定项目并校验项目格式；不依赖 roots/env。所有连接转交实例，继续使用公开 UUID、revision 和项目锁。新 Lumi 插件在子代理中的实际连接行为另验收 |
| Step 4 · 生图导入 | App 原生生图会落盘，独立 MCP 能按路径读取；已有对话图片可以作参考 | 把输出路径当宿主结果传入，不硬编码 savedPath/目录/命名。精确像素不可依赖，比例偏差应按现有计划柔性处理；立即 Asset Store 导入，保留真实尺寸/来源。CLI 端到端与复杂角色 MCP 参考仍待测 |
| Step 5 · MCP Apps | 后续 Step 4.5 已验证真实渲染、桥接及两种 App 内工具取图模式的六档加载 | 采用只读工具返回受控缩略图 data URI；保留完整文本与 Lumi 入口。loopback 当前失败，CSP 对照和写操作批准/拒绝仍未验证；8 MiB padding 是已测下界，生产大小沿用受控预览限制 |
| Step 6 · 插件 / skill / 安装 | 本地 marketplace 的 add/install 命令与新会话 MCP 可用；显式 Subagent 触发可行 | 使用绝对 CLI 路径；写清配置写入、项目信任、hook 信任及可能的宿主批准。skill 明确何时委派并保留串行路径；另测真实用户安装、升级幂等、CLI 登录后的完整流程 |
| Step 7 · 可选记录导入 | hook 可取得 cwd、session ID 与现存 rollout 路径 | 默认关闭，正常信任不能省略；补测 Stop、PostToolUse、中断/恢复和完整助手输出。格式适配须版本化、失败不阻断创作；路径为空/文件轮换或格式变化须降级，提供手动 import |

本步完成的是带边界的调研结论与复现材料。所有标为「未验证」的项都没有被视为已成立的产品能力。
