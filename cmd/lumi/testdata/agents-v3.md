# Lumi 项目

这是一个 Lumi 项目，与 Lumi 桌面版共用项目格式。
在 Codex 中，所有项目内容的读取与修改必须通过 Lumi 插件工具完成。
禁止直接读取或写入 project.sqlite（包括 -wal、-shm、-journal）、.lumi/、assets/。
STORY.md 是只读投影，可读取但不得直接修改；正文修改使用 Lumi 插件工具。

首次创作带页面生图时，先生成并拆分必要设定图，复用已有资产；“只生成第一页”只限制正文图。
只有用户明确要求纯文字、跳过设定图或限制全部生图次数时才调整，不自行添加额度限制。
生图前调用 image_target 查询比例和构图建议；角色、场景参考先用 read_image 放入对话。
页面 image_target 返回 setting_readiness；选择本页 setting_uuids 检查缺图，补齐后再读取参考。
页面生图先按插件技能渲染项目当前的基础规则与参考图说明，再 render_prompt chapter/section_image，
target=page；使用完整 generation_prompt 和 image_write_back，不自行从分镜另写生图提示词。
comic_storyboard 和 image_target 不能替代生图模板；生图与 import_image.prompt 保存同一完整提示词。
使用宿主自带生图能力，生成后立即把工具结果中实际提供的绝对路径传给 import_image。
图片保留期限未知；不得假设 savedPath、猜测文件名或扫描固定目录。比例偏差默认允许导入，
可按用户意愿选择 keep、居中 crop 或白色 pad；不要因比例偏差阻塞创作。

这些要求是行为指引。项目级 .codex/config.toml 的 permission profile 只限制
命令沙箱的写入，不限制读取；须先由用户信任项目，且可能被更高优先级配置覆盖。
不要替用户设置信任。Codex App 中的实际保护仍需用户人工确认。
