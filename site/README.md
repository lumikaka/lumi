# Lumi 官网与使用教程

此目录是 Lumi 官方网站与使用教程的独立 Hugo 项目，不参与 React 应用、Go 服务或桌面安装包构建。

站点包含简体中文和英文两套内容：中文首页位于 `/`，英文首页位于 `/en/`；使用教程分别位于 `/docs/` 与 `/en/docs/`。首页视觉参考 [Read Frog](https://www.readfrog.app/) 的留白、胶囊按钮和产品界面叙事，但品牌、文案、截图和实现均为 Lumi 自有内容。

## 环境要求

- Hugo Extended 0.164.0
- `jq`（仅用于构建产物校验）

检查 Hugo 版本：

```bash
hugo version
```

## 本地开发

以下命令均从 Lumi 仓库根目录运行：

```bash
hugo server --source site --disableFastRender
```

严格生产构建与路由校验：

```bash
hugo --source site --gc --minify --panicOnWarning
./site/scripts/check-build.sh site/public
```

构建产物位于 `site/public/`，不会提交到 Git。

## 相对路径与部署

页面导航、CSS、JavaScript、字体、图片和搜索索引由 Hugo 生成相对 URL。构建时只需传入部署位置对应的 `baseURL`，它同时用于 canonical、Open Graph、hreflang 和 sitemap：

```bash
# 域名根路径
hugo --source site --baseURL https://example.com/

# 任意子路径
hugo --source site --baseURL https://example.com/products/lumi/
```

不要在模板、Markdown 或前端资源中写死域名、`/lumi/` 前缀或根绝对资源地址。

## 内容约定

简体中文是默认语言。每篇教程的中英文文件使用相同 `translationKey`、slug 与权重，通过语言后缀关联，例如：

```text
content/docs/installation.zh-cn.md
content/docs/installation.en.md
```

教程面向 Lumi 使用者，只描述产品可见行为。API、数据库、任务队列和迁移等开发资料继续保留在仓库根目录的 `docs/`。

## 界面截图

`assets/images/screenshots/` 保存真实应用截图，首页与教程通过 `product-demo` 共用这些资源。更新 UI 后，中英文各五张图片应一起重新截取；保留文件名即可更新所有引用，Hugo 会自动生成 800px、1200px 的响应式图片。

2026-09-08 更新使用「小熊的月亮灯」项目与新版简洁工作区：

| 文件前缀 | 截图页面（相对于项目路径） |
| --- | --- |
| `overview` | 项目主页 |
| `story` | `story`，完整故事 |
| `premise` | `premise`，设定列表 |
| `comic` | `chapters/{chapter_uuid}/sections/{section_uuid}`，正文第 1 页 |
| `preview` | `chapters/{chapter_uuid}/preview`，全书视图 |

截图规格为 1440 × 900 CSS 像素、设备像素比 2，输出 2880 × 1800 WebP，质量 88。使用浅色界面，展开左侧导航栏、收起项目聊天区，滚动至页面顶部，等待字体和图片加载完成后截取视口，不包含浏览器工具栏。

简体中文界面保存为 `*.zh-cn.webp`，英文界面保存为 `*.en.webp`。仅切换界面语言，项目名称、故事正文和图片文字保留原文。示例的最后两页尚未生成图片，全书视图保留其真实占位状态；不要为了截图触发生成、恢复版本或修改项目内容。

替换图片时同时检查 `i18n/zh-cn.yaml` 和 `i18n/en.yaml` 的 `demo_*_alt` 是否与实际画面一致，并运行上面的严格构建和路由校验。`local` 演示目前由模板绘制，不对应截图文件。

## GitHub Pages

`Site Pages` workflow 仅支持手动触发。首次发布前：

1. 在 GitHub 仓库的 **Settings → Pages** 中将 Source 设为 **GitHub Actions**。
2. 在 **Actions → Site Pages** 中手动运行 workflow。

CI 使用 GitHub Pages 提供的实际站点地址构建，随后检查中英文关键路由、搜索索引和相对资源 URL，再上传 `site/public/`。
