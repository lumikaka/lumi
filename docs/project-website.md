# 项目网站生成

在项目的「导出」页面找到 **Website**，点击「生成网站」。首次成功后按钮变为「更新网站」。生成由用户手动触发，不会自动更新或发布到 Sites。

如果存在缺图，展开列表查看，并明确勾选「我确认仅导出已有内容」后生成。至少需要一张可用正文图片。生成期间可继续编辑；本次使用点击时冻结的快照，新的修改会让完成后的卡片继续显示「有更新」。失败、取消或中断后可「重试本次快照」，也可以重新生成当前内容。

成功后可「预览网站」或「打开输出目录」。输出固定在项目根目录：

```text
website/
  index.html
  rss.xml
  README.md
  static/reader.js
  static/style.css
  data/site.json
  data/site.js
  data/build.json
  media/<content-key>.jpg
```

所有生成资源使用相对引用，可以把整个 `website/` 目录复制到任意普通静态服务器或子路径部署，无需 Lumi、项目数据库、原始 `assets/` 或外部字体/CDN。`data/site.js` 与 `site.json` 是相同展示数据的两种封装；`build.json` 仅包含公开生成 UUID，用于目录恢复识别。

`website/` 长期保留，不受 ZIP/PDF 七天清理影响，与仓库的 Hugo 官网 `site/` 无关。它是 Lumi 管理的生成目录，直接修改生成文件可能被后续生成覆盖。需要 Codex 定制时请使用独立副本，再按需交给 Codex，通过 Sites 或其他静态托管服务发布。本功能只生成和本地预览。

Website 卡片中的「用 Codex 发布网站」提供完整提示词和一键复制，自动带入当前项目的 `website/` 路径；Codex 无法访问本机时，需一并提供文件夹副本。提示词要求使用独立副本、按 Sites 静态站点流程部署、用真实部署地址替换 RSS 的 `{site_url}`，并检查线上阅读和订阅链接。若首次部署后才得到地址，Codex 需替换后再次部署。复制失败时会选中提示词，允许手动复制。此按钮仅复制文本，不会由 Lumi 自动上传或发布。

中英文发布提示词均附带执行效率要求：按现有静态网站直接部署，按需读取工具说明，使用摘要输出和批量检查，避免重复验证与扩大排查。浏览器验证覆盖代表性阅读流程，资源可访问性由脚本批量检查；保留默认访问权限，授权验证仍受限时报告限制，不额外搭建代理。这些要求不减少文件保护、RSS 替换、上传范围和部署结果核验。

## 阅读与数据范围

网站根据项目形态自动使用两种界面：绘本采用书籍式首页、章节目录和单页翻阅，支持上一页／下一页、方向键和滑动翻页；条漫采用漫画式首页、话数目录和连续纵向画面，图片之间不插入页码标题或额外间距。条漫在手机上铺满阅读宽度，在桌面上居中限宽。点击图片可以打开大图。两种模式均适配手机和桌面。

仅采用当前选定图片版本及有效页面，保留封面、正文、封底顺序。空章节显示暂无图片，明确允许的缺页会在首页提示数量。站点只输出展示标题、公开章节 UUID、页角色、页码及压缩图信息，不输出历史版本、提示词、故事原文、聊天记录、内部 ID、源图路径或数据库。

## RSS 订阅

每次「生成／更新网站」都会同步输出 `rss.xml`，网站首页包含 RSS 自动发现标签和页脚入口。使用 RSS 2.0，带 Atom 自链接和条目更新时间扩展；不会额外生成 Atom 文件。

每个有可用图片的有效章节对应一条订阅项，按作品顺序输出标题、章节阅读链接、图片数量和首张压缩图片。空章节不进入订阅，缺图导出会在频道说明中提示遗漏数量。章节公开 UUID 组成稳定 GUID，不使用内部 ID；不含提示词、原始资产地址或项目数据库。

**发布地址使用 `{site_url}` 占位符。** 发布独立副本时，把 `rss.xml` 中全部占位符替换为包含协议和部署子路径的完整网站目录地址，末尾不加斜杠。例如 `https://example.com/my-book`，订阅地址就是 `https://example.com/my-book/rss.xml`。生成目录的 `README.md` 和 Website 卡片也包含此说明。网页自身的相对资源引用不需要替换；新的导出仍包含占位符，因此每次部署副本都要替换。

RSS 阅读器不会解析变量。替换前的文件是待部署模板；替换为完整 URL 后才适合正式订阅。这是为了遵循 [RSS 的绝对 URL 要求](https://www.rssboard.org/rss-profile#data-types-urls)，同时避免把 Lumi 本地预览地址或尚未确定的域名写入产物。替换值须使用已进行 URL 编码的地址，例如路径中的 `&` 写成 `%26`，不包含原始 XML/HTML 特殊字符。

首次收录时间、内容更新时间和摘要存入项目 SQLite 的生成 manifest。无变化时 RSS 内容和日期保持不变；改标题或图片会更新对应章节的内容时间，首次收录时间和 GUID 保持稳定；只改章节顺序不会让章节重新成为新条目。RSS 与网站使用同一冻结快照、校验和可恢复切换，失败或取消时一起保留上一版。旧版网站会提示「有更新」，重新生成即可补齐 RSS，已有图片仍按原缓存键复用。

## 图片与增量复用

- 经 Asset Store 读取，不修改原始资产。
- 固定最大宽度 **1600px**、JPEG **92**、白色透明背景合成、Catmull–Rom 等比缩放，小图不放大。条漫按宽度缩放，例如 2000×6000 → 1600×4800。
- 复用键是源图 SHA-256 与处理器预设 `width1600-jpeg92-white-catmullrom-v1` 的摘要。调整处理算法或参数时必须更新预设版本。
- 上一次成功生成的 manifest 保存在项目 SQLite。复用前比对压缩文件大小与 SHA-256；新增、替换、丢失、损坏或预设变化的图片才重新处理。同源同预设只输出一份 JPEG，标题／排序变化不重新压图。
- 只从上一版 `website/media/` 复用。构建中的临时目录和切换备份在成功／恢复后移除，不长期保留第二份压缩缓存。

## 任务、一致性与恢复

`website_generations` 通过内部自增 `task_id` 关联现有 `production_task_runs`，`project_websites.generation_id` 指向当前成功版本。任务类型是 `website_generation`，复用 River 调度、执行 fencing、幂等键、取消、重试及任务事件。数据库唯一索引限制同项目只有一个活动网站任务；工作锁还覆盖取消后的旧 worker 退出过程。网站任务预算为 30 分钟，队列 rescue 窗口大于该预算。

创建任务时在同一事务内重新读取导出内容，确认与请求阶段的快照相符；不符则返回内容已变化。重试沿用原快照。生成过程不锁住用户的内容编辑。

构建先写入 `.lumi/website-staging/<generation_uuid>/`，校验受管文件，再持久化原子恢复日志 `.lumi/website-swap.json`。切换时短暂把旧目录移到 `.lumi/website-previous/`，安装新目录后，在同一 SQLite 事务中提交当前版本、统计、成功时间、任务完成状态及完成事件。取消命令只能在提交之前生效。

项目重新打开、worker 启动前，以及重试前都会恢复：数据库未提交则恢复旧目录；已提交则清理备份。日志写入中断、替换中断和回滚清理中断都可重入。旧目录中的非受管文件会保留；受管但不再引用的文件随成功切换被清理。原始资产和外部目录不受影响。已有但不属于 Lumi 的 `website/`、符号链接或自定义文件冲突会拒绝接管，提示先移动自定义内容。

## REST 与实时更新

所有 API 使用统一 JSON 信封、`snake_case` 和公开 UUIDv7：

| 方法 | 资源 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/projects/:project_uuid/website` | 当前可导出状态、最新任务、最近成功时间、更新提示 |
| POST | `/api/v1/projects/:project_uuid/website/generations` | `{allow_missing_images, idempotency_key}`，返回公开任务 UUID |
| POST | `/api/v1/projects/:project_uuid/website/directory-openings` | 打开固定输出目录，不接收客户端路径 |
| POST | `/api/v1/projects/:project_uuid/production-tasks/:task_uuid/cancellations` | 复用任务取消 |
| POST | `/api/v1/projects/:project_uuid/production-tasks/:task_uuid/retries` | 复用快照重试 |
| GET | `/media/projects/:project_uuid/website/*` | 本地预览，只允许当前 manifest 中的受管文件 |

现有 `/api/v1/ws` 的项目频道发送变更提示，前端失效 `project-website` Query，再通过 REST 重读 SQLite。首次 join、重新 join、窗口重新聚焦沿用 `useProjectRealtimeSync` 校准，没有 HTTP 定时轮询。

## 验证与首版限制

自动测试覆盖：迁移保留既有任务事件、拒绝丢失网站记录的降级；无变化／标题／排序／单图替换／重复原图；缓存丢失和同大小损坏；缺图与明确同意；原选定版本和有效章节顺序；冻结快照、并发点击、取消、重试；切换的各个中断阶段、原子日志和回滚重入；保留自定义文件；API 信封及预览路径白名单；展示数据隐私；WS Query 失效；独立阅读器的翻页、键盘、触摸逻辑与章节切换。

RSS 专项验证覆盖 XML 与 HTML 双层转义、替换发布地址及子路径链接、章节筛选与顺序、稳定 GUID／日期、无变化重复导出、缺失／损坏修复、冻结快照、取消／提交失败回滚，以及预览 MIME 类型。

2026-10-23 验证结果：以下 7 个 Go 包完整测试通过，随后网站专项回归通过；`go vet` 通过；前端 416 项测试及生产构建通过。

```sh
go test ./internal/dbmigrate ./internal/production ./internal/jobqueue ./internal/httpapi ./internal/files ./internal/server ./internal/project
go vet ./internal/production ./internal/jobqueue ./internal/httpapi ./internal/files ./internal/server ./internal/project
pnpm --dir web test
pnpm --dir web build
```

已在普通静态服务器下检查 1440px 桌面与 390px 手机视口的绘本／条漫：图片加载正常，无横向溢出，无外部请求，无控制台错误。触摸逻辑有自动测试，尚未在实体手机上验证。

首版使用固定预设，不提供自定义模板、动画图保留、互动剧情逻辑、自动更新或发布。动画源图输出首帧，互动绘本作为静态图片阅读。若被冻结的原始资产在生成前被删除或破坏，任务会失败并保留上一版网站，需要恢复资产后重试或生成新的快照。站点依赖 JavaScript；本地预览需要 Lumi 运行，复制后的独立网站不需要。

可复现浏览器测试素材（指定一个新的空目录）：

```sh
LUMI_WEBSITE_PREVIEW_ROOT=/tmp/lumi-website-preview go test ./internal/production -run '^TestWebsiteBrowserFixtures$' -count=1
python3 -m http.server 8764 --bind 127.0.0.1 --directory /tmp/lumi-website-preview
```

访问 `/classic_picture_book/` 与 `/vertical_strip/`。测试素材是合成图片，不包含用户项目数据。相关验证不需要 Cargo 或 Rust 编译。
