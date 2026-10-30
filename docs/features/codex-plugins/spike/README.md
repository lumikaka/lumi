# 一次性 Codex 宿主探针

结论见 [`../spike.md`](../spike.md)。依赖：已安装 Codex CLI、Python 3.10+、git。没有 pip/npm/Cargo 依赖，不使用用户认证。

所有命令从 Lumi 仓库根目录运行。脚本在 `/tmp` 下创建新目录并打印路径；macOS 上解析为 `/private/tmp`。不会自动删除证据目录，也不会写用户真实 `~/.codex/config.toml`。

## 运行

```sh
codex --version
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py

# 可选：让真实 MCP 读取本地图片；可使用已提交的测试图
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --image "$PWD/docs/features/codex-plugins/spike/evidence/fox-original.png"

# 可选：比较 App bundle 中的 CLI
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/run_probe.py \
  --codex /Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex

# 项目信任、权限配置、hook 发现；打印独立 sandbox_command
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py

# 仅执行生成器提供的已审阅 hook；单次信任 bypass，不持久化信任
# 本地离线 provider 不会产生成功模型回复，5 秒后终止整个测试进程组
PYTHONDONTWRITEBYTECODE=1 python3 docs/features/codex-plugins/spike/config_hooks_probe.py \
  --run-reviewed-hooks
```

然后执行配置探针打印的 `sandbox_command`。它只向新建夹具目录的假数据库、假素材文件尝试写入、原子替换、删除和改名。若从另一层 Codex 沙箱运行，可能需要批准在外层之外启动被测 sandbox；不要用真实 Lumi 项目替换夹具目录。

生成器内部创建 marketplace 的全部 JSON，可作为最小插件。`run_probe.py` 使用真实 JSON-RPC stdio app-server，拒绝任何意外批准请求，不发起模型 turn。资源 HTML 的传输成功不等于实际渲染成功。

`call_probe.py ROOT reference_image '{}'` 通过同一临时插件返回 MCP 图片内容。它的输出是 JSON；让宿主转呈其中 image 块才会进入模型视觉上下文。工具结果本身没有自动给另一条 CLI thread 添加图片。

## 文件

| 文件 | 用途 |
| --- | --- |
| `run_probe.py` | 生成/安装最小插件；两个不同 cwd 的 thread；重复 inspect；图片读取；UI 工具关联和资源读取 |
| `probe_server.py` | 无依赖 stdio MCP；仅记录允许列表环境值和测试请求；提供 inspect/read_image/reference_image/show_app |
| `config_hooks_probe.py` | 生成 trusted/untrusted 项目和权限/hook 夹具，可选执行已审阅测试 hook |
| `hook_probe.py` | 记录测试 hook stdin 与 transcript 是否存在 |
| `write_probe.py` | 只应在生成器创建的临时夹具 cwd 执行 |
| `app.html` | 未完成 GUI 验收的 MCP Apps 探针；loopback、PNG data URI 和 tools/call 按钮 |
| `evidence/` | 本次实际结果；路径/PID/UUID/时间只属于此次运行 |

## 本次证据

| 文件 | 来源 |
| --- | --- |
| `stdio-image.json`、`stdio.jsonl` | PATH CLI `0.163.0` 的 app-server/RPC/MCP 记录 |
| `app-bundled-stdio-image.json`、`app-bundled-stdio.jsonl` | App bundle CLI `0.166.0-alpha.5.1` 同项记录 |
| `config-hooks.json`、`hooks.jsonl`、`hook-transcript-metadata.json` | CLI `0.163.0`，信任发现和真实离线轮次的两个 hook 输入；不含完整 rollout |
| `sandbox-writes.json` | CLI `0.163.0`，macOS Seatbelt 下的真实文件操作结果 |
| `image-generation.json` | 本次 App 三次内置生图的原始参数、返回字段名及落盘提示；未重复保存 base64 |
| `fox-original.png`、`fox-boots.png` | 真实 App 生成/参考编辑，1536×1024 PNG |
| `mcp-reference-result.json`、`mcp-reference.png` | 真实 stdio MCP image 结果及其原始 PNG，128×128 |
| `mcp-reference-ring.png` | MCP 图片转呈进入对话后的参考编辑，1254×1254 |
| `subagents.json` | 两次真实 Subagent 调用与返回报告的整理记录；不是 CLI rollout |
| `ImageGenerationItem.ts`、`HookEventName.ts` | `codex app-server generate-ts --out /tmp/...` 导出的 `0.163.0` 类型 |
| `app-inventory.json` | 当前 App 的 MCP Apps 通道存在且 tabs 为空；不含其他应用/浏览器资料，不代表渲染通过 |

App iframe、loopback、data URI 上限和 UI 回调没有成功观测记录，不能从这些文件推导为通过。所有 PNG 均为本次测试新建内容。
