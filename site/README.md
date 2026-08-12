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

## GitHub Pages

`Site Pages` workflow 仅支持手动触发。首次发布前：

1. 在 GitHub 仓库的 **Settings → Pages** 中将 Source 设为 **GitHub Actions**。
2. 在 **Actions → Site Pages** 中手动运行 workflow。

CI 使用 GitHub Pages 提供的实际站点地址构建，随后检查中英文关键路由、搜索索引和相对资源 URL，再上传 `site/public/`。
