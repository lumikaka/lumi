# MCP 矩形局部改图

`prepare_page_image` 支持从本页任意历史版本准备原尺寸局部编辑；`import_image` 根据签名凭据把补丁贴回历史基准，仅新增候选版。无新增界面；未提供局部参数时沿用整页流程。CLI 创作协议 v11；新客户端遇到旧后台会拒绝局部操作并要求升级，不会降级成整页导入。

## 工具调用

先 `pages get(include_versions=true)`，按 `image_versions.items[].version_no` 查找版本 UUID，用该项 `asset_uuid` 看图。矩形坐标相对完整页面，左上角为原点，不相对局部预览。

```json
{
  "page_uuid": "<本页 UUIDv7>",
  "base_variant_uuid": "<历史图片版本 UUIDv7>",
  "selection": {"x": 0.5331491712707183, "y": 0.2292817679558011, "width": 0.17403314917127072, "height": 0.058011049723756904},
  "additional_direction": "将上格左臂金环改为棕皮带配银色圆章"
}
```

局部模式不需要设定图，`setting_uuids` 省略或为 `[]`。返回 `reference_mode=local_edit`、原尺寸上下文裁图路径及提示词。选区向外取整，自动外扩选区长边的 50%（至少 16 像素，边界处截断），不缩小原图。准备凭据 `local_edit` 记录版本、Asset、原图 SHA-256、原尺寸、归一化选区、像素选区、裁图坐标及羽化宽度；全部参与摘要与 HMAC 签名。

查看裁图后，把唯一该路径和原样 `generation_prompt` 交给宿主 ImageGen。返回图须保留完整上下文构图和宽高比；允许整体分辨率变化。服务端按裁图尺寸对齐，比例偏差超过 2% 时拒绝。此约束不能识别模型在相同比例画布内移动主体；仍须实际看合成结果。

立即 `import_image`，传本轮 `image_write_back`、实际输出路径、完整原样 `preparation` 与提示词，`fit_mode=keep`。局部导入拒绝 crop/pad；仅写选区内部，并在选区内最多 8 像素羽化。PNG 保存避免 JPEG 重编码污染圈外像素，也保留 16 位 PNG 的圈外样本。候选返回 `adopted=false`；通过已有 `pages select_image` 与最新 revision 才会采用。候选新增会递增页面 revision，并用已有 WebSocket 变更提示通知 Web 重读。

## 自动验证

```sh
go test ./internal/production ./internal/textmcp ./cmd/lumi ./internal/httpapi
node --test cindy-plugins/tests/contract-drift.test.cjs cindy-plugins/tests/media.test.cjs cindy-plugins/tests/protocol.test.cjs
node scripts/build-skills.mjs --check
```

新增测试覆盖预览到原图坐标、浮点边界取整、选区边界与非法值、上下文原尺寸裁切、双倍模型像素对齐、非对称补丁位置、JPEG 基准保存为 PNG 后圈外零变化、16 位透明 PNG 圈外样本保真、签名篡改拒绝、历史基准候选与显式采用、重复旧 revision 拒绝，以及 MCP schema 到真实生产服务的完整路径。

## 原尺寸实测（2026-11-17）

来源为《火星公主β》022 章第 12 页第 6 版，通过 Lumi `pages` 和原始资产 HTTP 接口获取；没有读取项目内部数据库或资产路径。七个历史版本经接口复制到独立测试项目。

- 原项目：`01a1b122-0570-779d-9b65-ad0aacd1a55c`
- 章节：`01a1e76d-02f1-7799-85f8-ee9f55557c6e`
- 页面：`01a1e774-3925-73cc-b844-12fd70e85c71`
- 基准第 6 版：`01a1e7ba-9766-74be-9956-afb7c2e1ba21`
- 基准原图 SHA-256：`0d6543ba16e627eac7bf445f2817726893d2b9407443ec8a3455782d1293574f`
- 原图及合成结果：724 × 2172，原始 PNG 2,411,803 字节。
- 选区：`{x:386,y:498,width:126,height:126}` 原图像素；上下文：`{x:323,y:435,width:252,height:252}`。
- 原生 ImageGen 返回 1254 × 1254 完整裁图，Lumi 缩放回 252 × 252 后仅在选区内合成。
- 圈外变化 0 像素，圈内变化 15,086 像素。最终构建重新准备并导入为测试候选第 9 版，与首轮第 8 版候选 PNG 逐字节相同；仍采用第 7 版。
- 对比使用同一上下文坐标、两侧同为 2 倍最近邻显示；未对预览图进行贴回。
- 银章为银灰色金属，白亮高光与浅浮雕可辨，无明显黄金色；棕皮带贴合手臂透视，边缘、细缝线与皮肤接触阴影自然；2 倍对比未见明显矩形接缝。这里只报告本次具体成图的视觉观察，不以协议测试证明所有生成质量。

本机实测文件保存在 `output/local-image-edit/`（已在目录内忽略生成资产）：`source-manifest.json`、`prepared.json`、`imported.json`、`candidate-state.json`、`pixel-audit.json`、`before-v6.png`、`after-candidate.png` 和 `comparison-2x.png`。测试副本的采用版仍为第 7 版；原项目采用版、revision 与版本数量也另行通过接口重读核对。最终候选身份和逐像素计数以 `candidate-state.json`、`pixel-audit.json` 为准。

回归结果：Go 的 production、textmcp、cmd/lumi、httpapi 四个包全部通过；Cindy 契约、媒体、协议共 15 项测试通过；生成技能检查通过。自动测试包含合成数据，实际图片结果另由上述原尺寸实测和像素审计验证。

共享 Web 已用独立无头 Chrome 实际打开最终构建：正常保留侧栏、章页导航、图片、脚本和聊天区；724 × 2172 当前图片加载成功，标注“当前使用 · 第 7 稿”。点击既有“9 稿”入口打开历史列表，v9/v8 显示“使用此稿”，v7 显示“当前使用”；没有自动切换。截图与 DOM 记录为 `web-current.png`、`web-detail.png`、`web-before.json`、`web-candidate-detail.json`。
