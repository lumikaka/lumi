---
title: 安装 Lumi
description: 下载并安装 macOS Apple Silicon 或 Windows x64 桌面版，安全处理首次启动提醒。
translationKey: installation
slug: installation
docs_group: start
weight: 10
keywords: [下载, 安装, macOS, Windows, SmartScreen, 校验]
---

## 选择正确的安装包

前往 [Lumi Releases](https://github.com/lumikaka/lumi/releases)，在最新稳定版本中下载与你的系统匹配的文件：

| 系统 | 下载文件 | 说明 |
|---|---|---|
| macOS Apple Silicon | `Lumi-macos-aarch64.app.zip` | 适用于 M1、M2、M3、M4 等 Apple 芯片 Mac |
| Windows x64 | `Lumi-windows-x64-setup.exe` | 适用于 64 位 Windows 10/11 |

旧版 Intel Mac 当前没有桌面安装包。不要把 GitHub Actions 下载的外层 artifact 容器直接当作应用；正式 Release 页面提供的是可以直接使用的交付文件。

## 在 macOS 上安装

1. 双击 ZIP 解压，得到 `Lumi.app`。
2. 将 `Lumi.app` 移到“应用程序”文件夹，或保存在你信任的位置。
3. 在 Finder 中打开 Lumi。启动器会在本机启动 Lumi 服务，再用默认浏览器打开工作台。
4. 如果 macOS 因应用尚未公证而阻止启动，先确认下载来自本仓库 Release，并核对同版本提供的 SHA-256。随后前往“系统设置 → 隐私与安全性”，仅在确认来源后选择“仍要打开”。

{{< callout type="warning" title="不要分享访问地址" >}}
托盘菜单里的 “Copy Access URL” 包含当前运行实例的临时访问令牌，应像密码一样对待。不要把完整地址发送给其他人或粘贴到公开位置。
{{< /callout >}}

## 在 Windows 上安装

1. 运行 `Lumi-windows-x64-setup.exe`。安装程序使用当前用户模式，一般不需要管理员权限。
2. 如果 Microsoft Defender SmartScreen 显示“Windows 已保护你的电脑”，先确认文件来自 Lumi Release，并用同版本 `.sha256` 文件核对哈希。
3. 确认来源无误后再选择显示更多信息并继续运行。安装完成后从开始菜单打开 Lumi。
4. Lumi 会在本机启动服务，并用默认浏览器打开工作台。

Windows 安装程序目前没有 Authenticode 签名，因此 SmartScreen 提醒并不等于文件损坏；它也不应成为跳过来源和哈希检查的理由。

## 确认安装成功

成功启动后，你会看到“连接 AI 模型服务”页面。暂时不要创建项目，先进入下一篇教程完成模型服务配置。

如果关闭浏览器页面，桌面程序仍可能在菜单栏或系统托盘运行。可以从 Lumi 托盘菜单重新打开；选择退出才会同时停止本机服务。
