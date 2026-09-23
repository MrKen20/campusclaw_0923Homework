# AGENTS.md — CampusClaw 仓库规则

## 铁律：无规约不写代码

- 本仓库采用规约驱动开发（SDD）。**规约是契约，代码是派生物。**
- 任何功能改动必须先存在对应的 OpenSpec change（`openspec/changes/<change>/` 四件套齐全），再动代码。
- 规约含糊或缺失时，**先改规约（change 里的 delta），再改代码**；不得由实现方自行裁量扩张范围。
- 生成物若偏离规约，审查者引用 `specs/<capability>/spec.md` 中的 Scenario 原文驳回，不通过追加对话扩大范围。

## OpenSpec 流程

- 一次变更 = 一个 change：`proposal.md`（为什么/不做什么）、`design.md`（怎么做）、`specs/<capability>/spec.md`（做到什么算合格）、`tasks.md`（带 verify 的待办）。
- 五步：Propose → Review → Apply → Verify → Archive。
- **只动 change 目录**：`openspec/specs/` 是长期事实来源，只由归档更新，Apply 期间禁止回写。
- Apply 一次一个 task：实现 → 审查 diff → 运行该条 `verify` → 勾选 `- [x]` → 提交。禁止一次性生成全站。
- 勾选必须与 verify 实际运行结果一致；没跑就勾视为进度失真。

## 安全红线

- 认证、授权、班级隔离、上传校验全部在服务端（Go）强制执行；前端隐藏入口不构成访问控制。
- 口令只存 bcrypt 哈希；会话密钥、数据库凭据只来自环境变量，缺失必须启动失败，禁止内置默认值。
- 密钥不得进入仓库历史；若曾提交，必须轮换而不是只删提交。
- 角色与班级每次请求从服务端会话/数据库读取，不采信客户端声明。
- 上传目录不得作为静态资源暴露；文件读取必须过鉴权接口。

## 范围控制

- 动手前先复述 `proposal.md` 的范围与 Non-goals；Non-goals 内的功能（RAG 问答、对话助手、作业流程、JWT/OAuth、多副本等）一律不生成。
- 若发现自己额外生成了规约未声明的功能，停下来：先补规约 Non-goals，再删越界代码。
