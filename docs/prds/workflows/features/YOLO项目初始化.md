# 工作流 — YOLO项目初始化

## overview

该 Feature 使用 `yolo_project_initialization` Workflow 根据用户输入生成并初始化项目 Story 与后续创作资源。它把项目初始化拆成可观测步骤，允许中断、恢复和诊断，而不是把整次生成压缩成不可恢复的单一请求。

## data_model

YOLO Workflow 使用 `workflows.input_snapshot` 固化用户输入、Prompt 与模型，`workflow_steps` 保存各阶段的业务 Task 或资源 UUID。可选 `thread_id` 关联用户发起初始化时的对话，但 Workflow 生命周期独立。

## api

| 接口 | 方法 | 说明 |
|---|---|---|
| `/api/v1/projects/:project_uuid/workflows` | POST | 创建 `yolo_project_initialization` Workflow。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid` | GET | 读取初始化进度、步骤和公开错误。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/cancellations` | POST | 取消初始化。 |
| `/api/v1/projects/:project_uuid/workflows/:workflow_uuid/retries` | POST | 使用冻结输入重试。 |

## ui

| 页面 / 入口 | 说明 |
|---|---|
| 新建项目 YOLO 流程 | 以步骤状态展示初始化，失败或中断时提供受控重试。 |

## others

Workflow 只编排；总纲、Chapter、Premise 或漫画结果各自按所属 domain 的 revision、版本和幂等约束提交。
