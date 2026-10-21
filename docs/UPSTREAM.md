# 来源记录

研究与实现日期：2026-10-20。通过 GitHub API 再次核对时，原仓库 `main` 仍为 `fd031e136ae3d89147324c0d1b2aa65e838c21f5`。

## 官方格式

- [MiniMax H3 基础提示词指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md)
- [MiniMax H3 全参考提示词指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md)
- [官方 H3 提示词 Skill](https://github.com/MiniMax-AI/MiniMax-H3/blob/main/skills/h3-prompt-writing/SKILL.md)
- [托管 Context-IR API 文档](https://platform.minimax.io/docs/api-reference/video-generation-v2-h3-context-ir)

这些资料说明公开的提示词字段、引用、音频、对白和关键帧。我们没有取得 MiniMax 托管系统的内部实现、训练数据或模型。

## OpenH3-IR

参考：[ruashots/open-h3-ir](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)，固定提交 `fd031e136ae3d89147324c0d1b2aa65e838c21f5`，Python 包版本 0.4.1。

已将其确定性计划、素材编号/音轨配对、帧与画布网格、latent rows、完整词表 BPE、H3 校验规则、LoRA 合同、ComfyUI 远端操作、HTTP 请求合同和上传存储转换为原生 Go 能力。`compiler` 将自然语言与多模态读数通过类型化决策编译进统一 `Document`，再确定性渲染。

有意改变的机制是：用户意图不通过词法规则分类，改由注入模型的结构化合同解释；所有失败、有限重试、草稿政策和时长对齐均明确可见；部分观察不冒充完整媒体理解；固定对白和身份/替换/摄影机等不可变结构由宿主管理。命名不同的 Go 内部类型不被当成已经通过功能验收的证据。

验收直接运行原仓库的 991 个测试、23 个 controls，另保存实际函数输入与输出、原断言来源，以及六个原始模型评测需求和完整许可/创意语义语料。原文件来源、哈希、调用与覆盖矩阵在 [conformance](../conformance/README.md)。Python 只用于对照测试工具，不是 Go 包运行依赖；混合测试总通过数与实际执行 Go 的原节点数分别报告。

原项目采用 Apache-2.0。其 LICENSE/NOTICE 位于 [third_party/open-h3-ir](../third_party/open-h3-ir/NOTICE)，复制的 9 个 golden prompt 和 2 张原图保留字节哈希。完整 Qwen2.5 词表内嵌于 `tokens/data/vocab.json`，来源与许可证另在 [third_party/qwen25-tokenizer](../third_party/qwen25-tokenizer/NOTICE)。这些许可不额外授予 MiniMax 权重或其它上游素材的权利。
