# Lumi 通用 Agent 接入包

OpenCode、pi 和 Claude Code 使用同一份 [create-picture-book Skill](skills/create-picture-book/SKILL.md) 和同一个 stdio 服务：

```sh
/absolute/path/lumi mcp --host generic
```

Lumi 保存项目事实和版本，Agent 执行文字创作，`generate_page_image` 使用 **Lumi 已配置的图片服务与默认图片模型**生成并挂载页面，费用按该服务商计算。`web_url` 返回 Lumi Web 的审阅、手动编辑和服务商配置入口。无需 MCP Apps、宿主原生生图或 hook。设定图使用已有图片导入，或由用户在 Lumi Web 生成后重读；通用 MCP 不提供设定图生成工具。

Codex、Cindy 继续使用 [Codex 插件](../codex-plugins/README.md)、[Cindy 插件](../cindy-plugins/README.md)，各自独立验收，不改接通用模式。省略 `--host` 保持历史工具清单。旧 `--host claude` 已移除，请改为 `generic`。

## 准备和启动目录

使用带嵌入前端的 Lumi CLI。源码构建只需要 Go 和 Web，不运行 Rust：

```sh
pnpm --dir web run build
go build -tags embed_frontend -o build/lumi ./cmd/lumi
node scripts/build-skills.mjs --check
```

下面把 `/absolute/path/lumi` 替换为该二进制绝对路径，`/absolute/path/lumi-repo` 替换为本仓库绝对路径。在**绘本项目根目录**按下方示例配置并启动宿主。新作品从空目录开始，已有作品从包含 `project.sqlite` 的目录开始。MCP 绑定启动目录，切换作品时需要在目标目录重新启动连接。

加载 Skill 后调用 `project_overview`。新目录返回 `project_not_initialized` 是预期结果，接下来按[共同创作流程](#共同创作流程)创建草稿。已有作品会返回项目名称和 UUID，可据此核对绑定。

多个插件可以使用同一绘本目录。通用插件用 `lumi init --host generic` 补齐共享指引，只有与旧官方模板完全匹配的 `AGENTS.md` 才会升级。Codex 插件在自己的初始化步骤中管理该项目的 `.codex/config.toml`。宿主未自动加载 `AGENTS.md` 时，可显式读取。

下列文件是**合并片段**，不要覆盖已有配置。Skill 链接目标始终是同一个目录；也可复制该目录连同 `references/`。已有同名 Skill 时先核对来源，避免覆盖。Windows 可复制目录替代符号链接。

## OpenCode

以下为 OpenCode **2.x** 的 `opencode.json`；1.x 的 MCP 配置将服务器直接放在 `mcp.lumi` 下，不能混用两个版本的格式。以 `opencode --version` 和本机帮助为准。

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "lumi": {
        "type": "local",
        "command": ["/absolute/path/lumi", "mcp", "--host", "generic"]
      }
    }
  }
}
```

```sh
cd /absolute/path/my-book
mkdir -p .opencode/skills
ln -s /absolute/path/lumi-repo/agent-integration/skills/create-picture-book .opencode/skills/create-picture-book
opencode mcp list
opencode
```

在会话中让 `skill` 加载 `create-picture-book`，再调用 `lumi_project_overview`（启用 Code Mode 时通过 `tools.lumi.project_overview`）。核对返回的项目名称、公开 UUID 与预期目录。参见官方 [MCP 配置](https://opencode.ai/docs/mcp-servers/)、[Skill 加载](https://opencode.ai/docs/skills/)。

## pi

使用带**原生 MCP**的 pi 版本，先运行 `pi mcp --help` 确认。旧版不支持该命令时升级 pi，不为 Lumi 编写扩展。项目 `.pi/mcp.json`：

```json
{
  "mcpServers": {
    "lumi": {
      "command": "/absolute/path/lumi",
      "args": ["mcp", "--host", "generic"],
      "timeout": 180,
      "exposure": "direct"
    }
  }
}
```

```sh
cd /absolute/path/my-book
mkdir -p .pi/skills
ln -s /absolute/path/lumi-repo/agent-integration/skills/create-picture-book .pi/skills/create-picture-book
pi mcp list
pi
```

项目配置按 pi 自身的信任流程加载。使用 `/skill:create-picture-book` 加载 Skill；调用直接暴露的 `mcp__lumi__project_overview`（省略 `exposure` 时可通过宿主工具搜索／Code Mode 调用），核对项目名称与 UUID。生图可能等待约 90 秒，示例把请求超时设为 180 秒。参见官方 [MCP](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md)、[Skills](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md)。

## Claude Code

项目根目录 `.mcp.json`：

```json
{
  "mcpServers": {
    "lumi": {
      "command": "/absolute/path/lumi",
      "args": ["mcp", "--host", "generic"]
    }
  }
}
```

```sh
cd /absolute/path/my-book
mkdir -p .claude/skills
ln -s /absolute/path/lumi-repo/agent-integration/skills/create-picture-book .claude/skills/create-picture-book
claude
```

按 Claude Code 提示批准项目 MCP 服务，使用 `/mcp` 检查连接、`/create-picture-book` 加载 Skill，调用 `mcp__lumi__project_overview` 核对项目名称与 UUID。也可在该项目目录运行 `claude mcp add --transport stdio --scope project lumi -- /absolute/path/lumi mcp --host generic`，不要重复添加同名服务。参见官方 [MCP](https://code.claude.com/docs/en/mcp)、[Skills](https://code.claude.com/docs/en/skills)。

## 共同创作流程

1. **新建作品**时，在 `project_overview` 返回 `project_not_initialized` 后，调用 `project_init`，用 `idea` 传入用户的原始创意。草稿创建成功后，再在同一目录用同一 CLI 和环境运行 `lumi init --host generic --json`，补齐 `AGENTS.md` 和 `user-data/.gitkeep`。在空目录提前运行该命令会直接定稿默认规格。
2. **选择规格或续做**时，调用 `project_setup get`。若 `setup_status=draft`，按用户要求用 `project_setup update` 填写画风、绘本形式及该形式支持的参数，再用返回的新 revision 调用 `project_setup finalize`。已有作品若为 `ready`，直接沿用已定稿的规格和当前进度。

规格定稿后的创作流程为 `render_prompt → story_profile/chapters/settings/pages 写回 → image_target → generate_page_image → read_image → pages record_image_review → web_url → export_book/export_website`。工具参数与审阅证据见 Skill 引用的参考文档。

每次写入使用最新 `expected_revision`，冲突时重读。生图断线、超时或 `done=false` 时保留全部原参数与 `idempotency_key` 重试，避免重复计费。缺服务商时给出 `web_url(view=providers)`，由用户在 Lumi 配置，不把密钥交给 Agent。手动编辑之后重读事实和 revision。网站导出成功后交付输出目录、预览链接和原样 `publish_prompt`；发布仍需用户另行提出。

## 从旧 Claude 插件迁移

1. 保留项目、`CLAUDE.md`、`.claude/settings.json`、已有 Skill 自定义和应用数据。记录原 MCP 的二进制、`--environment`、`--data-dir`；通用入口必须使用同一环境和数据目录才能沿用图片服务配置。
2. 在 Claude Code 查看已安装插件，禁用旧 Lumi 插件（开发版通常为 `lumi-dev@lumi-dev-local`）；确认无需使用后可通过宿主卸载插件和对应 marketplace。仓库不再提供 `claude-plugins/`、`make claude-dev*`、安装脚本或 PreToolUse hook。用户自行添加的权限规则不会被迁移程序修改。
3. 按上例链接通用 Skill，注册 `lumi mcp --host generic`。删除旧插件提供的重复连接应通过 Claude Code 自身管理完成，不同时运行两份同名 Lumi Skill/MCP。开发环境在 args 后追加 `--environment development --data-dir /absolute/path/dev-data`，其他 CLI 命令使用相同参数。
4. 通用宿主需要创作协议 v10。遇到 `cli_upgrade_required`，先协调共享该数据目录的 Codex/Cindy/其他宿主，结束活动任务后，用新版 CLI 和原 environment/data-dir 执行 `lumi stop`，再重新连接；不要直接替换仍在工作的共享后台。
5. 在目标项目目录新开会话，加载 Skill 并调用 `project_overview`。核对原项目 UUID，再续做；无需重新初始化作品或生成已有图片。

实际验证范围与未实测项见 [验收记录](../docs/generic-agent-validation.md)。
