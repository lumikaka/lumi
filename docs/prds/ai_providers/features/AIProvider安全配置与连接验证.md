# AI Provider — AIProvider安全配置与连接验证

## overview

该 Feature 在单机应用库中安全管理 Cloudflare AI Gateway 与阿里云百炼等受支持 Provider。它保存 Provider 的公开身份、账户或 Workspace、默认文本/图片模型和激活状态，并把 API Key 用操作系统 Keychain 根密钥派生的 AES-256-GCM 密钥加密后写入 `site_settings`。

连接检查针对当前配置 fingerprint 执行。后续修改账户、模型、区域或密钥会令旧验证失效，因此项目运行时不会把已过期的检查结果当成 ready。

## data_model

`site_settings` 是当前事实表。Provider 公开投影包含 `uuid`、`provider_type`、展示名、默认模型、`configured`、`verified`、`active`、`ready` 与 `has_secret`；`api_key`、Authorization 和 Keychain 细节永不出现在响应、日志或 WebSocket payload。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/providers` | GET | 返回受支持 Provider 的安全公开投影。 |
| `/api/v1/providers/active` | GET | 返回当前 active 且 ready Provider。 |
| `/api/v1/providers/:provider_uuid` | GET | 返回单个 Provider 投影。 |
| `/api/v1/providers/:provider_uuid/connection-checks` | POST | 用当前配置执行连接验证。 |
| `/api/v1/site-settings` | GET / PATCH | 读取公开设置或提交可写 Provider 配置。 |
| `/api/v1/site-settings/resets` | POST | 重置允许的设置键，例如密钥。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| `/setup/*` | 首次配置 Provider、密钥和连接检查。 |
| `/settings/providers` | 修改 Provider 配置、默认模型、验证和激活状态。 |

## others

设置变更经 `/api/v1/ws` 的 `site_settings:updated` 提示刷新；客户端重新 GET Provider/Settings，不把事件 payload 当作配置事实源。
