# 来源与实现选择

## 固定上游

ContextIR 参考 [ruashots/open-h3-ir](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)，固定提交为 `fd031e136ae3d89147324c0d1b2aa65e838c21f5`，对应 Python 包版本 0.4.1。固定提交用于稳定格式、算法和测试来源，不表示以后自动跟随其 `main`。

原项目的确定性计划、素材编号与音轨配对、帧与画布网格、latent rows、完整词表 BPE、H3 校验规则、LoRA 合同、ComfyUI 远端操作、HTTP 请求合同和上传存储已实现为原生 Go 能力。高层 `compiler` 将自然语言和结构化观察编译进统一 `Document`，随后由 `h3` 确定性渲染。

来源文件、固定 SHA-256 和不可变上游 URL 记录在 [source_manifest.json](../conformance/source_manifest.json)。原始需求清单和 Go 测试映射见 [测试说明](UPSTREAM_TESTS.md)。来源清单是静态追溯信息，不是当前源码的一次测试执行报告。

## 公开格式资料

- [MiniMax H3 基础提示词指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md)
- [MiniMax H3 全参考提示词指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md)
- [官方 H3 提示词 Skill](https://github.com/MiniMax-AI/MiniMax-H3/blob/main/skills/h3-prompt-writing/SKILL.md)
- [托管 Context-IR API 文档](https://platform.minimax.io/docs/api-reference/video-generation-v2-h3-context-ir)

这些资料描述公开的提示词字段、引用、音频、对白和关键帧。ContextIR 没有取得 MiniMax 托管系统的内部代码、训练数据或模型，也不宣称其内部实现或生成质量等价。

## Go 实现的明确差异

- 用户意图由显式结构化输入或注入模型的类型化合同解释，不复制上游依赖词表的意图分类机制。
- 观察、用户要求与创意补充分别记录来源；部分视频观察与 ASR 转写不会被表述为完整音视频理解。
- 固定对白、镜头边界、摄影机闭集、素材绑定与不可变文字由宿主管理。
- 失败、有限修复、草稿政策、澄清和时长对齐均显式可见，调用方选择不会被偷偷替换。
- 公共 HTTP 合同使用 ContextIR 的版本与 IR 表示；命令行采用 Go 的实际接口。兼容原业务能力不要求复刻 Python 命令、内部常量或 dataclass。

## 许可证与保留素材

OpenH3-IR 使用 Apache-2.0，其原有 [LICENSE](../third_party/open-h3-ir/LICENSE) 与 [NOTICE](../third_party/open-h3-ir/NOTICE) 保留原文。`third_party/open-h3-ir/testdata` 保留其九份 golden prompt、两张参考图片及公开格式测试输入；其他原始媒体 fixture 的路径和哈希见各自 manifest。

完整 Qwen2.5 词表内嵌于 `tokens/data/vocab.json`，其 [来源声明](../third_party/qwen25-tokenizer/NOTICE) 与 [许可证](../third_party/qwen25-tokenizer/LICENSE) 单独保留。模型权重、托管服务和调用方媒体的许可不由本库的 Apache-2.0 授予。完整分类见 [第三方说明](../THIRD_PARTY_NOTICES.md)。
