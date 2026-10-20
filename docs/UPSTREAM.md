# 来源记录

研究与初始实现日期：2026-10-20。

## 官方格式

- https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md
- https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md
- https://github.com/MiniMax-AI/MiniMax-H3/blob/main/skills/h3-prompt-writing/SKILL.md
- https://platform.minimax.io/docs/api-reference/video-generation-v2-h3-context-ir

这些资料定义公开提示词字段、引用、音频、对白与关键帧表示。本项目没有获得 MiniMax 托管 Context-IR 的内部代码、模型或训练数据。

## 参考项目

OpenH3-IR: https://github.com/ruashots/open-h3-ir

固定研究提交：`fd031e136ae3d89147324c0d1b2aa65e838c21f5`。

借鉴范围包括 models.py 的分层概念、plan.py 的素材与音轨绑定经验、grid.py 的运行时数值规则、golden/ 与 controls 的测试方法。本项目使用独立 Go 类型与实现，没有引入该项目的 Python 运行时。

有意重做的部分：词法意图路由、整篇模型写作绕过 Plan、自动语义 repair、文本截短、失败回退草稿、隐式时长调整。规则测试只支持结构合同，不被当作生成质量证据。

该项目标示 Apache-2.0；其 NOTICE 还列出其它上游。为保留参考出处，附上该固定提交的 LICENSE/NOTICE 于 third_party/open-h3-ir/。这不会为 MiniMax 模型或其它上游素材额外授予许可。
