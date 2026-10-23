# 网站与 RSS 发布说明 / Website and RSS

此目录由 Lumi 生成。请先复制到独立目录，再交给 Codex 定制或发布；直接修改可能被下次生成覆盖。Lumi 不会自动发布此网站。

## 发布前替换 RSS 地址

每次生成／更新网站都会同步生成 `rss.xml`，每个有图片的有效章节对应一条订阅条目，包含标题、阅读链接、首张图片、首次收录时间及内容更新时间。

`rss.xml` 中的 `{site_url}` 是发布地址占位符，RSS 阅读器不会自动解析。部署独立副本时，将其中所有 `{site_url}` 替换成完整的网站目录地址，包含 `https://` 和实际部署子路径，末尾不要加 `/`。

例如网站发布在 `https://example.com/my-book/`，替换值为 `https://example.com/my-book`。订阅地址为 `https://example.com/my-book/rss.xml`。请使用已进行 URL 编码的地址，例如路径中的 `&` 写成 `%26`；不要把 XML/HTML 特殊字符直接放入替换值。

替换完成前，`rss.xml` 是待部署的订阅模板，不能保证订阅器兼容。不要使用 Lumi 本地预览地址、本机路径或临时测试地址作为公开地址。重新导出后的发布副本也需要进行这一步。

## 独立部署

把整个目录放到普通静态服务器即可，图片与网页资源均在目录内。网页中的相对资源引用无需替换。`rss.xml` 与网站应一起更新，避免订阅条目和图片不一致。

For deployment, copy this folder, then replace every `{site_url}` in `rss.xml` with the full public site directory URL (including HTTPS and any subpath, without a trailing slash). Use a URL-encoded address, such as `%26` for an ampersand in a path, without raw XML/HTML special characters. The token is not resolved by feed readers. Deploy the website and feed together. Repeat the substitution for each new Lumi export.
