---
title: 在 Codex App 中创作
description: 用一句提示词安装 Lumi 插件，在 Codex App 中从想法创作并导出本地绘本，无需 API key。
translationKey: codex
slug: codex
docs_group: start
weight: 15
keywords: [Codex, 插件, 安装, 绘本, 本地, 生图]
---

Lumi 插件让你在 Codex App 对话中完成故事、角色设定、分页、画面、审阅和导出。无需 API key，写作和生图使用 Codex 自身额度，作品保存在本地。生成所需的文字和参考图片仍会交给 Codex。

首版仅支持 macOS Codex App，提供 Apple Silicon 与 Intel CLI；不支持 Codex TUI 或 IDE 扩展。安装入口从首个包含 Codex 资产的 Lumi Release 起可用。真实 App 的一句话安装和自主整本创作仍待人工验收，不能承诺零批准。

## 一句话安装

在已登录的 Codex App 中粘贴：

> 请阅读 https://raw.githubusercontent.com/lumikaka/lumi/main/docs/features/codex-plugins/install.md，按文档为我安装 macOS Codex App 的 Lumi 绘本插件；安装后告诉我实际使用的 codex 路径和版本、安装结果及如何新开会话开始创作。

下载二进制、写用户安装目录和 `~/.codex`、注册 marketplace 和启用插件可能需要宿主批准。批准次数取决于你的权限设置。安装器不要求 API key，也不会替你信任项目。

**安装完成后，在 Codex App 中新开会话才能使用插件。** 为每本绘本选择一个本地文件夹，以它为工作目录。首次初始化后，由你信任项目，再新开会话核对写保护；生成配置文件本身不代表保护已经生效。

## 从一个想法开始

新会话中可以说：

> 用 Lumi 给 5 岁孩子做一本 8 页横版水彩绘本：小狐狸把迷路的星星送回天空。细节请你决定，直接完成整本并导出 PDF 和包含全部文字的 ZIP。

也可以要求“故事写完先给我审阅”“每张图确认后再继续”。没有指定时，模型会在规格定稿前，以及故事、设定和代表页完成后停顿审阅。

图片生成后会立即保存到项目，角色和场景可使用设定图作为参考。宽高比略有差异时，可以保留原图、裁切或补边。PDF 使用图片版面，可编辑文字随 Markdown/ZIP 交付。

## 继续、升级与卸载

额度不足或对话中断时，在同一目录新开会话说“继续这个 Lumi 绘本”；模型读取已有进度，再补缺失部分。不要自己修改项目内部的数据文件。

升级时重新运行最新安装脚本，完成后新开会话。卸载会保留你的绘本与 Lumi 应用数据。具体命令、故障排查及人工验收记录见 [完整安装指南](https://github.com/lumikaka/lumi/blob/main/docs/features/codex-plugins/install.md)。
