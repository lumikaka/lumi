---
git_commit_message: 'feat(site): 完成 Lumi 官网与双语使用教程'
plan_state: finished
---

# Lumi 官网与双语使用教程实施计划

## current_status

- `site/` 已初始化为独立 Hugo Extended 0.164.0 项目，简体中文为默认语言，英文位于 `/en/`；当前没有主题、layout、正文或站点素材，因此尚不能输出 HTML 首页。
- 官网不参与 `web/`、Go 二进制或桌面安装包构建；`.github/workflows/site-pages.yml` 负责手动构建并发布 `site/public/`。
- 产品事实与用户向文案目前集中在根目录 `README.md`；现有 `docs/` 主要是开发、架构和 PRD 文档，不能直接作为官网教程发布。
- 可复用品牌资产只有黄色枕头与蓝色星星图标，尚无适合官网的产品截图、社交分享图或双语教程插图。

## overview

- 为 Hugo 项目实现完全自定义的营销首页与教程主题，不引入第三方 Hugo 主题或前端框架。
- 视觉明确参考 [Read Frog](https://www.readfrog.app/) 的大留白、近黑粗标题、胶囊主按钮、柔和彩色悬浮卡片、真实产品界面演示和长页分段叙事；不得复制其品牌素材、页面文本或具体插画。Lumi 使用现有黄色枕头、蓝色星星和绘本画面形成独立品牌识别。
- 首版同时交付中文和英文：首页为 `/`、`/en/`，教程为 `/docs/`、`/en/docs/`，两种语言的页面、导航、搜索结果与截图一一对应。
- 所有站内链接、图片、字体、CSS、JavaScript、语言跳转和搜索索引均使用 Hugo 的相对 URL 能力生成，不绑定域名或仓库子路径。构建产物应可在域名根路径或任意子路径部署；`baseURL` 只用于 canonical、sitemap、Open Graph 等需要绝对地址的构建元数据。
- 首页主转化路径为“下载 Lumi”进入 GitHub Releases，次路径为“查看使用教程”进入当前语言的教程首页；不增加在线注册、价格、博客或云端工作区入口。

## information_architecture

### 公共路由

| 中文 | 英文 | 页面用途 |
|---|---|---|
| `/` | `/en/` | 产品官网首页 |
| `/docs/` | `/en/docs/` | 产品介绍、教程总览与推荐学习路径 |
| `/docs/installation/` | `/en/docs/installation/` | macOS Apple Silicon、Windows x64 安装与首次启动 |
| `/docs/providers/` | `/en/docs/providers/` | 配置阿里云百炼或 Cloudflare AI Gateway |
| `/docs/first-picture-book/` | `/en/docs/first-picture-book/` | YOLO 与手动模式创建第一本绘本 |
| `/docs/story-and-chapters/` | `/en/docs/story-and-chapters/` | 故事简介、章节、导入、续写与版本 |
| `/docs/premise-assets/` | `/en/docs/premise-assets/` | 角色、场景、道具和参考图管理 |
| `/docs/storyboards-and-images/` | `/en/docs/storyboards-and-images/` | 分镜、参考资产、图片候选与当前画面 |
| `/docs/preview-and-export/` | `/en/docs/preview-and-export/` | 连续预览、缺图处理与 ZIP/PDF 导出 |
| `/docs/local-projects/` | `/en/docs/local-projects/` | 本地项目目录、移动、备份和隐私边界 |
| `/docs/troubleshooting/` | `/en/docs/troubleshooting/` | 启动拦截、模型配置、失败任务和常见问题 |

- 中文与英文文件使用相同 translation key、slug 和权重；语言切换始终进入当前页面的对应翻译，不回退到语言首页。
- 教程左侧导航分为“开始使用”“完成第一本绘本”“项目与排障”三组；分组与排序由教程 front matter 驱动，桌面和移动端复用同一数据源。
- `/docs/` 不复制仓库开发文档。教程只描述用户可见行为；内部数据库、API、UUID、队列和 migration 等实现细节保持在现有开发文档中。

### 首页内容顺序

1. 顶部导航：Lumi 标识、产品能力、创作流程、教程、GitHub、语言切换和下载按钮。
2. Hero：中文标题“把一个想法，做成一本真正能翻阅的绘本”；副文案说明 Lumi 是专注绘本创作的本地优先 AI 工具；主按钮“下载 Lumi”，次按钮“查看使用教程”。英文使用自然意译而非逐字翻译。
3. 产品主视觉：真实 Lumi 工作台截图置于柔和窗口框中，周围使用“故事自己定”“角色不跑偏”“过程可修改”“作品留在本地”四张悬浮卡片。
4. 六步创作流程：故事想法、剧情与章节、角色与场景、漫画分镜、生成画面、预览导出。
5. 四个交替图文功能段：故事与章节、设定资产、可编辑分镜与图片候选、本地保存与版本恢复；每段只突出一个用户收益并配对应截图。
6. 创作方式：并列说明 YOLO 快速创作和手动创作，强调两者最终都能继续编辑。
7. 本地优先说明：项目保存在用户选择的本机目录；仅在调用模型时向用户选择的服务商发送完成生成所需的内容。
8. 常见问题：是否需要绘画基础、支持的平台、支持的模型服务商、项目保存位置、能否重新生成或恢复旧版本。
9. 末尾 CTA 与页脚：再次提供下载和教程入口，并链接 GitHub、Releases、教程与中英文切换。

## visual_design

- 使用暖白纸张背景 `#f7f5ef`、白色表面 `#ffffff`、墨黑文字 `#171715`、次级文字 `#68645b` 和浅褐分隔线 `#e7e1d6`。品牌色使用 Lumi 黄 `#f3c86b`、Lumi 蓝 `#315e91`，装饰色限制为浅黄、浅蓝、薄荷和淡紫。
- 自托管 Inter Variable 字体及 OFL 许可证；中文回退到 `PingFang SC`、`Noto Sans CJK SC`、`Microsoft YaHei` 和系统无衬线字体，禁止依赖外部字体 CDN。
- 内容最大宽度 1180px；首页主标题使用 `clamp(3rem, 7vw, 5.5rem)`、紧凑行高和轻微负字距。正文默认 17–18px、行高不低于 1.65。
- 主按钮为近黑背景的全圆角胶囊按钮，次按钮为浅色表面；卡片使用 18–28px 圆角、1px 浅边框和低对比度阴影。链接、按钮和卡片必须具有清晰的 hover、focus-visible 和 active 反馈。
- 悬浮装饰仅使用 CSS transform 和 opacity：桌面端缓慢上下浮动，交互过渡 160–220ms；`prefers-reduced-motion: reduce` 时关闭位移、滚动显现和非必要过渡。
- 首页在 768px 以下改为单列，截图置于文案之后，悬浮卡片收进主视觉边界且不得产生横向滚动。导航折叠为菜单按钮，下载 CTA 保持可见。
- 教程桌面布局为 272px 左侧固定导航、最大 760px 正文、240px 右侧页内目录；宽度不足 1180px 时隐藏右侧目录并在正文顶部提供折叠目录，低于 768px 时左侧导航改为可关闭抽屉。
- 教程沿用官网设计 token，但降低装饰密度；代码块、提示框、步骤列表、截图说明和前后页导航使用统一组件。首版只实现浅色主题。
- 为带有 `active`、`aria-current`、`aria-expanded` 或 `aria-pressed` 的交互元素显式编写组合 hover 状态，并置于基础状态之后，避免选中状态覆盖 hover 反馈。

## implementation

### Hugo 主题与资源管线

- 在 `site/layouts/` 建立 `baseof`、首页、教程 section、教程 single 和 404 模板；把站头、站尾、语言切换、图片框、教程侧栏、页内目录、搜索框和前后页导航拆为 partial。
- 在 `site/assets/` 维护 Sass、JavaScript、字体和图片。使用 Hugo Pipes 编译 Sass、压缩并 fingerprint CSS/JavaScript；页面只引用生成后的相对 permalink，不直接引用构建目录文件。
- `hugo.yaml` 保留中文默认语言和英文子目录，增加相对 URL、输出格式、菜单/markup、搜索索引和 minify 所需配置；关闭无使用场景的 taxonomy 与 RSS 输出，保留 sitemap 和 robots。
- 站内跳转在模板中使用 `.RelPermalink`、`relURL`、`relLangURL`，Markdown 内部链接使用 `ref`/`relref` shortcode。不得写死域名、`/lumi/` 前缀或以 `/assets` 开头的资源地址。
- canonical、Open Graph、sitemap 和 hreflang 从构建时 `baseURL` 与页面 permalink 生成；它们允许为绝对 URL，但不得参与页面运行时导航。

### 双语内容

- 首页结构化文案由按语言拆分的数据文件驱动，首页模板不得出现成段硬编码中英文文本。
- 教程使用 Markdown page bundle；每个教程包含 title、description、translation key、导航分组、weight、截图和可搜索关键词。
- 中文教程以现有 `README.md` 的用户向说明为主事实源，PRD 和实现代码只用于核对当前行为。英文保持术语统一：Story、Premise、Chapter、Storyboard、Section、YOLO 等产品术语与应用界面一致。
- 每篇教程包含目标、开始前准备、编号步骤、结果确认和“下一步”；故障类内容使用“现象—原因—处理”结构。不得加入尚未实现的能力或承诺时间表。

### 教程搜索与导航

- Hugo 为每种语言输出独立的教程搜索 JSON，字段固定为 `title`、`description`、`headings`、`content`、`url`、`group`；索引只包含当前语言的已发布教程。
- 使用原生 JavaScript 实现本地搜索，不接入外部搜索服务。搜索对标题和标题层级赋更高权重，对正文执行大小写归一化与连续子串匹配，兼容中文无空格检索；最多展示 8 条结果。
- 桌面端和移动端均可通过搜索按钮打开原生 dialog；支持 `Ctrl/Command + K` 打开、方向键选择、Enter 跳转和 Escape 关闭。无结果、加载失败与 JavaScript 禁用时均提供明确降级状态。
- 教程页使用 Hugo 自动 Table of Contents；左侧当前页使用 `aria-current="page"`，章节抽屉与搜索 dialog 打开时管理焦点，并在关闭后把焦点还给触发按钮。

### 产品截图与品牌素材

- 创建只用于截图的虚构演示项目“月亮邮差”，内容不得来自真实用户或受版权保护的现有作品。演示项目至少包含故事简介、两个章节、主角与场景设定、六个分镜、图片候选和可导出预览。
- 分别在中文和英文界面截取：首页/创建方式、故事与章节、设定资产、分镜与图片、连续预览与导出五组画面。截图统一使用 1440×900 视口，裁切为 16:10，并保留足够安全边距供响应式裁剪。
- 截图前清除或遮盖本机用户名、绝对路径、Provider 密钥、访问令牌、项目 UUID、任务 UUID和无关调试信息。最终只提交压缩后的 1600px WebP 母版，由 Hugo 生成 800px 与 1200px 响应式版本。
- 将现有 Lumi 图标复制为官网自包含品牌资源，并补充 favicon、Apple touch icon，以及中英文各一张 1200×630 社交分享图；不从 `web/dist/` 或第三方站点热链资源。

### SEO、可访问性与性能

- 每页提供独立 title、description、canonical、Open Graph、Twitter Card 和双向 hreflang；404 页面保留当前语言导航并提供首页和教程入口。
- 使用语义化 `header/nav/main/article/aside/footer`、唯一 H1、跳过导航链接、可见焦点、44px 最小触控区域和符合 WCAG AA 的文字对比度。所有内容截图必须有描述用户目的的 alt，装饰图使用空 alt。
- 首屏只加载 Hero 所需图片，其余截图 lazy-load；所有图片声明 width/height 和 `srcset/sizes`，避免布局跳动。JavaScript 仅负责导航、搜索和轻量交互，正文与核心导航在 JavaScript 关闭时仍可访问。

## tests_and_acceptance

- 运行 `hugo --source site --gc --minify --panicOnWarning`，构建必须无模板错误、缺失翻译、重复 destination 或资源处理警告。
- 分别用域名根路径和带子目录的测试 `baseURL` 构建，确认首页、十个中文教程入口、英文镜像、404、CSS、JavaScript、字体、图片和搜索索引都能从生成目录独立打开；模板与 Markdown 源文件不得硬编码生产域名或 `/lumi/`，生成 HTML 仅允许 canonical、hreflang、Open Graph 和 sitemap 等构建元数据反映当次测试 `baseURL`。
- 自动断言 `/index.html`、`/docs/index.html`、`/en/index.html`、`/en/docs/index.html` 及全部教程 `index.html` 存在；检查中文页与英文页的 canonical、hreflang 和语言切换互相对应。
- 在 1440×900、1024×768、390×844 三档视口验收首页和教程：无横向滚动、无遮挡导航、截图不失真、文档侧栏/抽屉状态正确、右侧目录按断点切换。
- 验收搜索的中文标题、中文正文、英文关键词、无结果、索引加载失败、键盘导航和当前语言隔离；验收教程目录锚点、前后页、移动菜单、语言切换和所有下载/GitHub 外链。
- 验收 `prefers-reduced-motion`、键盘全流程、focus-visible、dialog 焦点归还、ARIA 当前/展开状态和组合 hover 状态。
- 对首页和一篇教程执行 Lighthouse 桌面与移动检查；Accessibility、Best Practices、SEO 目标不低于 95，Performance 目标不低于 90，若截图体积导致未达标则先调整响应式尺寸与压缩率，不删除核心内容。
- 更新 Site Pages workflow，使 CI 使用与本地一致的严格生产构建并在上传前检查关键路由；最终人工检查 GitHub Pages 预览或任意静态服务器下的根路径与子路径部署。

## boundaries

- 本计划只修改 `site/` 与 Site Pages workflow，不改动 React 应用、Go API、数据库、桌面打包或产品运行时路由。
- 首版不包含博客、价格页、用户评价、安装量统计、在线账户、遥测、评论系统、深色模式、PWA 或自动识别平台后直链最新安装包。
- 下载按钮统一指向 `https://github.com/lumikaka/lumi/releases`，避免静态站依赖易变化的 Release 文件名；平台、签名与首次启动提醒在安装教程中说明。
- 不配置 CNAME、DNS 或固定生产域名。站点发布位置由部署环境提供，页面代码保持相对路径与子路径安全。
