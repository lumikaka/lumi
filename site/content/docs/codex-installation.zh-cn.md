---
title: 安装 Codex 插件
description: 复制一句安装提示词，在 macOS Codex App 中安装 Lumi 绘本插件，无需配置 API key。
translationKey: codex-installation
slug: codex-installation
docs_group: start
weight: 14
keywords: [Codex, plugin, 插件, 安装, 提示词, macOS]
---

Lumi 绘本插件让你在 Codex App 对话中创作和导出绘本。写作与生图使用 Codex 自身额度，无需额外配置 API key，绘本项目保存在本地。

## 安装前准备

- 使用 **macOS 上的 Codex App**，并登录账号。当前提供 Apple Silicon 和 Intel 版本，不支持 Codex TUI 或 IDE 扩展。
- 确保可以访问 GitHub，安装过程需要下载 Lumi 的正式发布文件。

## 一句话安装

点击“复制提示词”，将下面的完整内容粘贴到 Codex App 对话中并发送：

{{< copy-prompt >}}
请阅读 https://raw.githubusercontent.com/lumikaka/lumi/main/docs/features/codex-plugins/install.md，按文档为我安装 macOS Codex App 的 Lumi 绘本插件；安装后告诉我实际使用的 codex 路径和版本、安装结果及如何新开会话开始创作。
{{< /copy-prompt >}}

Codex 会读取安装指南并执行安装。下载文件、写入用户安装目录和 `~/.codex`、注册插件市场及启用插件时，可能会请求批准；根据界面提示处理即可。

安装结束后，查看 Codex 返回的安装结果。正式版插件名称为 `lumi@lumi-local`。

## 安装后开始创作

1. 为绘本选择一个本地文件夹，作为项目目录。
2. **在 Codex App 中新开会话**，以该文件夹作为工作目录。安装前的会话不会自动加载新插件。
3. 告诉 Codex 你想用 Lumi 创作什么绘本；具体示例见{{< doc-link "codex" >}}在 Codex App 中创作{{< /doc-link >}}。

首次初始化绘本后，需要由你信任项目，再新开会话核对写保护。安装器不会替你设置信任，生成配置文件本身也不代表保护已经生效。

## 升级与安装问题

升级时，按[完整安装指南](https://github.com/lumikaka/lumi/blob/main/docs/features/codex-plugins/install.md)重新运行最新安装脚本，完成后新开会话。指南也包含卸载步骤；卸载插件会保留绘本与 Lumi 应用数据。

如果下载失败，先检查网络和最新 Release 是否包含 Codex 插件安装资产。安装入口需要对应发布文件，不能仅凭仓库中存在源码就完成安装。
