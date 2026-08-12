## API 风格

REST API，路径格式遵循资源化设计，按语义使用 GET、POST、PUT/PATCH、DELETE 等 HTTP 方法。

应用级实时通信统一使用 `/api/v1/ws` 和 `topic/event/payload/ref/join_ref` 信封，实时 payload 只允许公开 UUIDv7，不得包含内部 `id`。

## 数据库表 ID 规范

- 数据库内部：字段名使用 `id`。主键、外键、JOIN 及关联关系均使用 `bigint` 自增 ID。
- Web/API 外部：字段名使用 `uuid`。URL、JSON、前端交互及开放接口均使用 UUIDv7，不得暴露内部 `id`。

## 技术栈

- 后端：Go 1.25、Echo v4、GORM v2、SQLite
- 前端：React 19（JavaScript）、Vite 7、React Router 7、TanStack Query 5、Sass、pnpm

`site/` 是使用 Hugo 构建的 Lumi 官方网站与文档项目，独立于 Lumi 应用的构建与打包链路。

不要在本地运行 Cargo 或 Rust 编译、检查及测试命令，因为本地磁盘空间宝贵；这些命令只允许在 CI 中运行。

## API JSON Response Contract

所有 API 端点必须返回统一的 JSON 信封格式，字段命名使用 `snake_case`。

**成功响应：**

```json
{ "success": true, "data": <object | null> }
```

**失败响应：**

```json
{ "success": false, "data": null, "error": { "code": "...", "message": "...", "details": "..." } }
```

### 单对象

`data` 直接为对象本身，不要包裹在 `{ item }` 或 `{ items }` 中。

### 列表

`data` 为 `{ items: [...] }`。

### 按页码分页

`data` 为 `{ items, pagination, filter_groups? }`，其中 `pagination` 为 `{ per_page, current_page, last_page, total }`。

### Cursor 分页

请求参数：`{ before, after, limit }`。
`data` 为 `{ items, cursor_pagination, filter_groups? }`，其中 `cursor_pagination` 为 `{ per_page, next_cursor, prev_cursor, has_more }`。

## 常见问题

为带有 `selected`、`active` 或 `aria-pressed` 等状态的按钮编写 hover 样式时，必须显式定义组合状态（如 `[aria-pressed="true"]:hover`）并放在基础状态之后，避免同等优先级的状态规则因源码顺序覆盖 hover 反馈。
