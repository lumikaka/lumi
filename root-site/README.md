# LumiKaka 根首页

这是 <https://lumikaka.github.io/> 的静态源码。唯一维护位置是 [lumikaka/lumi 的 root-site 目录](https://github.com/lumikaka/lumi/tree/master/root-site)。

- `index.html`：中文首页、内联 CSS、页面元信息；无需 JavaScript 或构建工具。
- `assets/`：Lumi 品牌标识与真实产品截图，初始文件来自 Lumi 官网的 `site/assets/images/`。
- `.nojekyll`：直接发布静态文件，跳过 Jekyll 处理。
- `deploy/root-pages.yml`：发布 workflow 的源文件，安装到发布仓库的 `.github/workflows/root-pages.yml`。

Lumi 产品官网与教程仍由 `site/` 中的 Hugo 项目管理，地址为 <https://lumikaka.github.io/lumi/>。根首页是产品入口，不参与 Lumi 应用构建。

## 修改与预览

从 Lumi 仓库根目录运行：

```bash
python3 -m http.server 8080 --bind 127.0.0.1 --directory root-site
```

打开 <http://127.0.0.1:8080/>。也可以直接打开 `index.html`。

首页的文字、链接和样式直接编辑 `index.html`；截图替换 `assets/lumi-preview.webp`，如果尺寸变化，一并调整 HTML 中的 `width` 和 `height`。这份截图是独立发布的文件，更新官网截图不会自动更新根首页。

发布前检查桌面与手机宽度的布局，以及官网、文档、下载和 GitHub 链接。

## 发布

1. 将修改提交并推送到 Lumi 仓库的 `master` 分支。
2. 在 **lumikaka/lumikaka.github.io → Actions → Root Pages → Run workflow** 中选择 workflow 分支 `main`，保持 `source_ref` 为 `master` 并运行；也可使用：

   ```bash
   gh workflow run root-pages.yml --repo lumikaka/lumikaka.github.io --ref main -f source_ref=master
   ```

3. Workflow 从公开的 **lumikaka/lumi** 仓库读取指定版本，只打包 `index.html`、`.nojekyll` 和 `assets/`，并在根首页仓库通过 GitHub Pages 部署。无需跨仓库写权限，也不需要 Deploy Key 或个人 Token。
4. 在 [目标仓库 Actions](https://github.com/lumikaka/lumikaka.github.io/actions/workflows/root-pages.yml) 查看发布结果。每次运行的摘要记录实际发布的 Lumi 源码 commit。

发布仅手动触发。回滚时，将 `source_ref` 指定为包含 `root-site/` 的历史 commit 或 tag，再运行 workflow。

## 首次配置或迁移账号

1. 创建公开仓库 `lumikaka/lumikaka.github.io`，默认分支为 `main`。
2. 将 `deploy/root-pages.yml` 复制到目标仓库的 `.github/workflows/root-pages.yml` 并提交。目标仓库只保留发布 workflow 与说明，不维护第二份首页源码。
3. 在目标仓库 **Settings → Pages** 中选择 **GitHub Actions**。
4. 确保 Lumi 仓库已提交并推送 `root-site/`，然后运行一次 `Root Pages`。

当前组织禁用了 Deploy Key，因此采用由发布仓库读取公开源码的方式。自动凭据只用于发布仓库自身的 Pages 部署。若 Lumi 仓库以后改成私有，需先调整源码读取授权。

## 管理约定

- 使用普通目录，不使用 Git submodule。
- 根首页 workflow 在 `lumikaka.github.io` 仓库运行；`site-pages.yml` 在 Lumi 仓库运行，两者独立。
- 页面与图片修改只需提交 Lumi 源码并运行发布。修改 `deploy/root-pages.yml` 后，还需要将该文件同步到发布仓库的 `.github/workflows/root-pages.yml`。
- 不在根首页引入框架、包管理器或构建步骤。增加产品时，沿用现有静态结构并补充实际链接。
- 部署仓库、账号或域名改变时，同步修改 workflow、页面 canonical/Open Graph 信息和产品链接。
