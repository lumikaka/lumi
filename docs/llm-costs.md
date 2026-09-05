# 本地调用费用估算

Lumi 在 AI 设置中提供模型价格目录，在项目「LLM 日志」中显示单次估算、当前筛选范围合计、模型／场景分组和历史补算。费用只覆盖已取得必需用量并找到完整价格规则的调用；未计价数量始终单独展示。金额为本地估算，不含免费额度、套餐、促销、充值手续费，不替代供应商账单。

## 价格规则

全局 `lumi.sqlite` 的 `model_prices` 保存不可变规则版本。模型全名、Provider 类型、实际请求地域和请求类型精确匹配；用户覆盖额外绑定 Provider UUID。Provider 当前由站点设置提供身份，因此这里的 UUID 是配置身份快照，不是跨表外键。覆盖优先，撤销后恢复内置目录；内置目录升级只新增内置版本，不修改用户版本。创建覆盖时用 `expected_uuid` 做并发检查。

设置界面可直接从内置价格创建覆盖，也可为未知模型添加规则。Token 单价按百万填写，图片单价按张填写；所有单价都是最多九位小数的十进制字符串。输入档位使用单次输入总量，区间下界不含、上界包含；下界零包含零用量。规格留空表示该规则适用于所有规格，存在重叠档位时拒绝保存。

2026-09-05 内置目录覆盖百炼北京 `qwen3.7-plus`，以及北京、新加坡、法兰克福、东京的 `qwen-image-3.0` 和 `qwen-image-3.0-pro`。采用公开标准价；未核实的渠道、控制台专属价格和部署范围存在歧义的条目保留「待配置」。来源为[百炼价格目录](https://help.aliyun.com/zh/model-studio/model-pricing)和[上下文缓存](https://help.aliyun.com/zh/model-studio/context-cache)。来源网址及核验日期随规则保存。

## 计算与持久化

`internal/pricing` 用整数和 `math/big` 计算，`1 单位货币 = 10⁹ 纳单位`。每个费用分项四舍五入到纳单位后求和；项目 SQLite 保存 `INTEGER`，API 返回十进制字符串。不同币种分别汇总，溢出报错，不使用浮点或自动汇率。

`Begin` 在真正调用前冻结价格版本、模型、地域、规格。`FinishAtomic` 将状态、已知用量、价格快照、分项和合计写入同一个项目事务。费用读取不依赖全局价格库。缓存已包含在输入总量内，只减一次；未返回用量与明确返回零分别保存。计价失败不会改变生成任务结果。

每次重试都是一条独立调用。失败或取消保留已取得用量；图片下载或后续保存失败也保留已生成数量。百炼 Pro 使用供应商 `output_image_type` 确认分辨率档位，不根据像素尺寸猜测计费档位。相关响应字段见[图片生成文档](https://help.aliyun.com/zh/model-studio/text-to-image)。

Cloudflare 转发的 Responses 支持两种配置：按张的整次请求估算，或父模型与图片工具分别按 Token 估算。后者要求供应商明确返回工具模型及完整的文本／图片／缓存用量分项；不将父模型用量再当作工具用量。缺项时整次调用仍为未完整计价。父模型与图片工具分别收费的依据见 [OpenAI 图片生成指南](https://developers.openai.com/api/docs/guides/image-generation)。

## 历史补算

在日志页面选择筛选条件，打开「历史费用补算」，明确勾选价格版本（含地域），先预览再执行。预览将日志集合、可证实用量、规则及计算结果持久化到项目内的 `llm_cost_backfills` / `llm_cost_backfill_items`；它们是补算作业记录，不是累计余额表。

执行每批最多 200 条，只条件更新仍未计价的终态日志。预览之后新增的调用不会被纳入，改价不会改变预览结果，重复执行不会再次更新已计价日志。中断后可从最近预览继续。完成结果标记「历史估算」，不提供覆盖重算。旧日志缺失的地域必须通过用户选择规则补充；不读取当前 Provider 配置推断过去环境。旧日志默认零不能证明供应商返回了零，不能据此补算。

## REST 与实时同步

全部接口使用统一 JSON 信封、`snake_case`，对外资源标识均为 UUIDv7。

| 方法与路径 | 作用 |
| --- | --- |
| `GET /api/v1/model-prices` | 全部价格版本及当前激活状态 |
| `POST /api/v1/model-prices` | `{rule, expected_uuid}` 创建覆盖版本 |
| `DELETE /api/v1/model-prices/:price_uuid` | 撤销覆盖 |
| `GET /api/v1/projects/:project_uuid/llm-cost-summary` | 全部匹配日志的合计、数量、分组 |
| `GET /api/v1/projects/:project_uuid/llm-cost-backfills` | 最近五个持久化预览 |
| `POST /api/v1/projects/:project_uuid/llm-cost-backfills` | `{filter, price_uuids}` 冻结预览 |
| `POST /api/v1/projects/:project_uuid/llm-cost-backfills/:backfill_uuid/applications` | 提交下一批补算 |

费用汇总复用日志的 Provider、模型、场景、状态、请求类型、关键词与 scope 筛选。`from`（含）和 `to`（不含）接受 RFC3339 时间，合计不受分页影响。日志详情包含 `cost_details` 的价格快照、用量和费用分项。未计价的 `cost_amount` 是 `null`。

调用完成或补算批次提交后通过 `llm_log:changed` 使日志、费用汇总及补算查询失效；价格变更通过系统主题的 `model_price:changed` 提示重读。首次 join、重新 join 和窗口聚焦复用现有校准机制。没有 HTTP 定时轮询。
