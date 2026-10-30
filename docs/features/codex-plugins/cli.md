# Lumi CLI：本地服务托管

`lumi` 为终端和后续 Codex App 插件托管独立的本地服务。一个实例可同时打开多个项目；关闭终端、某条 MCP 连接或 Codex 会话不会结束实例。

## 构建与命令

```sh
make build
./build/lumi serve
./build/lumi status --json
./build/lumi stop
```

`make build` 同时生成 `build/lumi`、`build/lumi_web`、`build/lumi_ctl`；前两者内嵌完整 web UI。`make build-linux` 生成对应的 Linux amd64 文件。直接 `go build ./cmd/lumi` 可构建 API 服务；不带 `embed_frontend` tag 的生产二进制在首页返回 503 并提示运行 `make build`。开发环境未内嵌 UI 时代理现有 Vite 开发服务器。

| 命令 | 行为 |
| --- | --- |
| `lumi serve` | 返回已有 CLI 实例；没有实例时启动独立后台进程，等待就绪后返回。使用 `127.0.0.1` 上系统分配的空闲端口，输出地址也是 web UI 入口。 |
| `lumi status` | 按需读取 CLI 地址、PID、环境及已打开项目。不会启动服务，也不探测桌面版。 |
| `lumi stop` | 正常关闭 CLI，等待 HTTP 请求、项目 Runtime / SQLite 关闭和实例锁释放；已停止时幂等成功。不会停止桌面版。 |

参数放在子命令之后：

| 参数 | 默认值与含义 |
| --- | --- |
| `--environment production\|development` | 默认 `production`，**不采用 shell 的 `APP_ENV`**；开发必须显式传入 `development`。 |
| `--data-dir PATH` | 优先于 `LUMI_DATA_DIR`；均未指定时沿用 `internal/platformpath` 的环境规则。相对路径转成绝对路径。 |
| `--json` | stdout 仅输出一条 JSON 信封，适用于三个命令。 |
| `--timeout 90s` | 启动、停止及等待并发命令启动锁的总等待上限；超时返回错误。 |
| `--help` | 显示用法。 |

```sh
lumi serve --environment development --data-dir /absolute/path/to/lumi-dev
lumi status --environment development --data-dir /absolute/path/to/lumi-dev --json
lumi stop --environment development --data-dir /absolute/path/to/lumi-dev
```

实例按「数据目录 + environment」识别。macOS / Linux 默认目录为 `~/.lumi`（生产）、`~/.lumi_dev`（开发）；Windows 沿用平台实现的 `dev.lumi.Lumi` / `dev.lumi.Lumi-dev`。显式为两个环境指定同一目录时仍是独立实例，但会共用该目录里的应用库。

CLI 忽略继承的 `APP_ADDRESS` 和 `FRONTEND_URL`，使用自己绑定的地址；其余配置沿用 `internal/config`，包括显式 `DATABASE_DSN`，默认数据库仍为该数据目录下的 `lumi.sqlite`。后台日志追加到 `{data_dir}/cli-{environment}.log`，创建权限为 `0600`。启动失败会指出日志路径。

## 退出码与 JSON

| 退出码 | 含义 |
| --- | --- |
| `0` | 命令成功，包括已经停止时再次 `stop`。 |
| `1` | 配置、启动、访问、关闭失败或等待超时。实例仍持有锁但无法确认身份时也失败，不会猜测 PID 后发送信号。 |
| `2` | 命令或参数错误。 |
| `3` | `status` 确认没有运行中的 CLI 实例，包括崩溃留下的过期发现信息。 |

运行中的 `serve --json` 与 `status --json` 使用相同结构：

```json
{
  "success": true,
  "data": {
    "endpoint": "http://127.0.0.1:51327",
    "pid": 12345,
    "environment": "production",
    "nonce": "<64 个十六进制字符>",
    "running": true,
    "open_projects": []
  }
}
```

`open_projects` 是已有 REST 项目 Summary 数组，包含公开 `uuid`、`name`、`root_path`、`open`、`status`、时间及可选绘本信息，不包含数据库内部 ID。`status` 是按需快照；客户端仍通过 `/api/v1/ws` 变更提示使查询失效，再经 REST 重读事实状态。

停止状态的 `data` 为：

```json
{
  "endpoint": "",
  "pid": 0,
  "environment": "production",
  "nonce": "",
  "running": false,
  "open_projects": []
}
```

`status` 返回成功查询信封并退出 `3`；`stop` 返回相同状态并退出 `0`。错误使用 `{ "success": false, "data": null, "error": { "code": "...", "message": "...", "details": "..." } }`，命令错误码为 `invalid_arguments`、`configuration_failed` 或 `instance_unavailable`。

## 发现与实例生命周期

CLI 独立发现文件：`{data_dir}/cli-{environment}.json`。

```json
{
  "endpoint": "http://127.0.0.1:51327",
  "pid": 12345,
  "environment": "production",
  "nonce": "<64 个十六进制字符>"
}
```

文件通过同目录 `0600` 临时文件写入、fsync、rename 原子发布，不含凭证。退出时仅删除 nonce 仍属于本实例的发现文件。nonce 是公开的实例代际标识，用于区分旧记录、端口复用和替换实例，不是鉴权凭证。

`cli-{environment}.start.lock` 串行化 `serve` / `stop`；后台进程全程持有 `cli-{environment}.lock` 的 OS 独占锁。锁文件保留，**不要删除锁文件来解锁**；进程退出或崩溃时内核自动解锁。并发启动者等待同一实例。发现记录须通过 `GET /api/v1/cli-instance` 校验完整身份，不能只按 PID 存活或某端口健康判断。

仅 CLI 注册管理资源：`GET /api/v1/cli-instance` 返回与 `status` 相同的 data；`DELETE /api/v1/cli-instance` 用公开 nonce 的 `If-Match` 头避免停止已替换实例。缺少或不匹配时返回 409 `cli_instance_changed`。`stop` 发送正常关闭请求后等待生命周期锁释放，不对发现文件中的 PID 发送信号。

崩溃遗留记录在生命周期锁释放后由 `serve` 替换。如果实例仍持有锁但 HTTP 不可用，`serve` 等待至超时后报错，不启动第二个实例。启动期间仅做有超时的就绪探测；没有新增业务状态的 HTTP 定时轮询。

## 鉴权和浏览器来源

- CLI 不使用 token / cookie 鉴权，启动子进程时移除继承的 `LUMI_DESKTOP_ACCESS_TOKEN`，不读取或保存桌面会话凭证。
- 所有实例在路由前检查原始 `Host`，只接受 `127.0.0.1:<本实例端口>`、`localhost:<本实例端口>`、`[::1]:<本实例端口>`。错误域名、其他端口、缺少端口均返回 403 `untrusted_host`；`X-Forwarded-Host` 不能绕过。覆盖 API、media、静态页和协议路径。
- 写请求缺少 `Origin` 时允许本地非浏览器客户端访问；携带时仍须匹配 `FRONTEND_URL` 的回环别名，否则返回 403 `untrusted_origin`。WebSocket `CheckOrigin` 保持原样。
- 桌面版设置了启动器 token 时，`/api`、`/media` 继续要求原有桌面 cookie，健康检查等既有例外保持原样；无 cookie 仍返回 401。CLI 不关闭、绕过或复用这套鉴权。

从 `status` 获取地址后可直接使用既有 REST API：

```sh
curl http://127.0.0.1:51327/api/v1/open-projects
curl -X POST http://127.0.0.1:51327/api/v1/open-projects \
  -H 'Content-Type: application/json' \
  -d '{"root_path":"/absolute/path/to/project"}'
```

## 桌面共存、应用库与项目锁

CLI 不探测、不复用桌面实例，不发布或替换旧 `mcp-{environment}.json`。CLI 专属实例锁不影响桌面启动。同一环境默认共用数据目录、`lumi.sqlite` 和最近项目索引。应用库 migration 从初始化驱动到关闭 Runner 全程持有数据库旁的 `.migrate.lock`，最长等待 60 秒；另一个进程已迁移时正常接受 no-change。未增加 migration，也未改变应用库或项目库 schema。

项目仍由 `.lumi/project.lock` 的 OS 锁独占。桌面和 CLI 可以各自打开不同项目；同一项目不能同时被两个实例打开。冲突返回 HTTP **423**、错误码 **`project_locked`**，`error.details` 展示锁文件中的占用主机/PID，并给出释放方式：

1. 在桌面版关闭该项目，或通过所属实例的 `DELETE /api/v1/open-projects/:project_uuid` 显式关闭。
2. CLI 占用时等待空闲回收，或执行相同环境、数据目录下的 `lumi stop`。
3. 对方崩溃后内核自动释放锁；残留 lock 文件不代表仍被占用。

CLI 空闲宽限期为 **15 秒**，每秒检查一次。在无请求租约、WebSocket project presence、queued/running 后台工作后开始计时；工作结束后重新计满宽限期。项目页面的 realtime presence 会继续持有项目，需要关闭页面或显式关闭项目。桌面默认仍为 5 分钟。`stop` 会关闭所有 CLI 项目并释放锁，桌面可立即重新打开。

本步遵循 [`spike.md`](spike.md) 的实测结论：同一 Codex 会话可有多条 MCP 连接，不同会话也有独立进程，因此实例和项目锁不绑定 MCP PID。与本步需求没有冲突。旧 MCP、Tauri 和后续插件步骤未在本步接入；桌面 web UI 同时查看 CLI 写入同一项目仍属后续设计范围。
