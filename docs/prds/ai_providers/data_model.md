# AI Provider — 数据模型

## 实体关系

```text
应用库
site_settings ──> Provider 公开投影（uuid、类型、默认模型、ready、active）
                         │
                         └──(UUIDv7)──> 项目库的 model settings 与冻结执行资源
```

Provider 不与项目库建立内部外键。项目表保存公开 Provider UUIDv7，跨库配置和密钥只存在应用库。

## 表：site_settings

当前应用级 Provider 配置的事实表。

- `id` — INTEGER PRIMARY KEY AUTOINCREMENT，仅内部使用
- `key` — TEXT NOT NULL UNIQUE，配置键
- `value` — 合法 JSON；敏感值保存 AES-GCM envelope 而非明文
- `updated_at` — DATETIME NOT NULL

Provider 相关键包括 active Provider、各 Provider 的 UUID、账户或 Workspace、区域、默认文本/图片模型、密钥、验证时间和配置 fingerprint。API 永远不会读取回密钥明文。

## 数据生命周期

1. 首次读取 Provider 时补齐稳定 UUIDv7 身份。
2. 保存配置后使旧验证 fingerprint 失效。
3. 连接检查成功才写入 verified 状态和当前 fingerprint。
4. 激活操作只接受 ready Provider；项目运行时从公开投影读取默认模型。
