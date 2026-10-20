# 实现与验收状态

更新日期：2026-10-20。目标是独立 Go package 实现公开的 Brief → Context-IR → H3 编译流程，并用 OpenH3-IR 固定提交的原始测试验证。它不是 MiniMax 托管 Context-IR 的内部实现，没有托管模型或成片质量等价证据。

旧 `v0.1.0` 是早期基础版本，不能代表本次实现。后续代码在 `codex/readiness-audit` 分支，旧标签不改写。运行时仅依赖 Go 标准库，没有 Python、CLW、数据库或队列依赖。

## 实际提供的能力

| 能力 | 生产实现与验收入口 |
| --- | --- |
| 自然语言高层编译与修订 | `compiler`：类型化模式、职责、替换、内容权限、参考许可、镜头结构、受约束写作、固定内容宿主回填；`Compile` / `Refine` / 独立显式 `Draft` |
| 模式、引用和音轨 | 五种 H3 模式、上游全部 16 个素材职责、身份/姿态/包装排版分离、源视频编辑/续写、主体放入/替换、音轨复制与属性引用区分、音视频配对 |
| 用户控制 | 固定对白与可见文字、歌唱与复用音轨 cue、跨镜分片逐字拼接、镜头与切点锁定、摄影机闭集、四档创意、七个导演及自定义导演、LoRA 注册与权重验证 |
| 唯一 IR 产物 | 根 `Document`、严格 JSON 与 Schema、不可变输入保护、观察覆盖与来源；同一 Document 确定性派生 H3 prompt、manifest、准备指令、预算与哈希 |
| 检查与预算 | `h3check` 原规则/等级/修复提示、帧网格、画布与 latent rows；`tokens` 使用完整 Qwen2.5 词表进行精确 BPE，不以字符估计 token |
| 离线评分与回归 | `evaluate` / `cmd/contextir-score`：最终产物评分、聚合、保留测量缺失状态的历史基线、比较与失败门卫；篇幅/警告数/差异度只作报告 |
| 真实媒体 | `media` 的真实文件头、ffprobe、ffmpeg 抽帧；显式 frame/card cache；真实像素观察；可注入 ASR；facts-only 感知器通过有 Fact 原文出处的类型化步骤构造卡片 |
| 模型和计量 | `deepseek` 与 `chatcompat`，显式模型/端点/结构化模式；有界合同修复；成功、失败、感知及修订调用进入同一用量账本，未知用量明确标记 |
| 可选宿主接口 | 标准库 HTTP handler、上传配额/在途预留/TTL/LRU、版本记录、待澄清状态、修订前依赖检查；`comfy` 图操作与远端 HTTP；可复验 CLI |

`Refine` 使用原始输入和观察重新编译。修改目标尺寸/时长时重新计算派生准备指令，不把上轮结果当作新的用户硬约束；不会修改调用者的资产切片或旧结果。固定对白和未修改的约束保留。执行 seed/effort 的变化必须由实际适配器明确实现。

最终叙事还在特定类型化风险职责下经过独立 `semantic_review`。审核必须引用实际片段才能报告矛盾，植入/替换身份的实际出场另有结构检查。审核不是完备性证明；它的真实调用、拒绝、修复和失败费用均保留，不把审核次数隐藏进一次写作调用。

## 验收证据的含义

完整原始 Python 基线是 **42 个测试文件、991 项**，另有 **23 个 controls、17 个 falsification mutations**。这些数字首先描述原项目，不等于 Go 已通过相同数量的独立测试。

原始输入、断言源码/行号、参数和实际调用分别保存在 `conformance/fixtures`、`matrix.json` 与不可覆盖的历史报告中。原断言桥接逐项记录哪些函数实际进入 Go，哪些仍执行 Python；组合回放也保留边界。独立 Go 测试另外执行权威 Document 的完整编译、保存重放、错误与并发路径。精确数字、当前失败和纯 Python 清单见 [逐项验收报告](UPSTREAM_TESTS.md) 与 [覆盖并集](../conformance/reports/coverage_union.json)。覆盖并集不是通过数，也不是整条流程等价声明。

真实 DeepSeek 验收使用上游的原输入；model-safe 输入与 held-out expected 分文件，模型不读取答案。完整六例使用原始 PNG 的真实像素，16 职责批次使用原测试明确提供的合成 cards，后者不能称为真实媒体感知。mode、scope、licence 的独立批次、每次实际 request/schema/output、失败、修复和用量均保留；失败不由下一轮覆盖。`compiled` 只说明记录的验证路径通过，还需检查实际身份、动作、固定文字与画风。当前各轮结果以 [验收报告](UPSTREAM_TESTS.md) 为准。

早期三个自编案例仅保留在 [evals](../evals/README.md) 作开发历史，不作为上游功能验收。JSON 有效、机械规则通过或那三个例子成功，都不能证明自由文本语义已经正确。

最终产品源码的本地 `go test ./...`、`go test -race ./...`、`go vet ./...`、gofmt 与 diff check 全部通过，逐条命令和输出见 [本地验证](../conformance/reports/local-validation-final-test-io.json)。冻结的 279 文件快照已独立重建，产品二进制 SHA-256 相同；后续变动仅为测试桥 DTO 投影和测试报告临时目录，不把测试修改混称为新的真实模型推理。普通 `go test` 不写仓库审计目录，下载后的模块也可运行测试。

## 明确的策略差异

- 自由文本通过类型化模型理解，不移植上游的关键词意图分类器；未知或矛盾结果不能靠词表猜测修补。
- 默认在未解决的机制选择处返回 `needs_input`。仅显式 `ClarificationContinue` 可沿已记录选择发布，并保留问题和警告。竖图首帧与横画布的冲突不能靠一个通过状态消除。
- 已提供文字同时被禁止时默认拒绝。明确选择 `ScopeConflictSuppliedWins` 才采用源项目的已提供内容优先策略，并同时记录冲突与解决；结构化 `SilenceAll` 仍不可覆盖。
- `Silent` 禁止新增配乐；复制已提供 BGM 是另一项职责。全面禁止音乐与禁止全部声音分别由硬约束表达。
- 帧对齐只在显式 `FramePolicySnapUp` 下进行，并报告名义/实际时长；默认 exact。遗漏 seed 保持未指定，不悄悄填源项目的 7。
- 原生首尾帧使用官方 ComfyUI 的 `first_frame` / `last_frame`，不沿用源旧 manifest 的错误 socket 名称。
- 非法模型协议、传输失败、缺感知和超限均明确失败。默认不返回草稿；可观察草稿必须显式选择。固定内容和已确定画风由宿主保存，避免后续写作重新抄错。

## 仍不能声称已验证的部分

1. 没有运行真实 H3 视频推理和成片对照，无法据 IR 测试证明人物一致性、动作质量、音画同步或成片文字保真。`comfy` 提供源项目同范围的图/HTTP 能力；它不自动执行 manifest 的缩放、裁剪、转码、socket 注入或 LoRA loader 配置。
2. 抽帧视频仅有部分视觉覆盖；ffprobe 元数据与 ASR 不能证明听过音色、音乐或节奏。默认感知覆盖要求不放宽，调用方必须明确接纳部分覆盖或注入相应能力。
3. 结构、Fact 引用与原文 quote 能检查来源关联，不能数学证明任意自然语言都完整蕴含于请求/观察。多个语义检查和原始测试也不能换算成总体可靠率。
4. 公开 H3 协议没有明确编码的边界（例如可见文字中的嵌套 ASCII 双引号、独立画外歌唱）继续明确拒绝，不自行发明可能误解的格式。
5. 已推送提交 `71a4bb5` 的 GitHub Actions 两个 job 因账户付款或 spending limit 未启动，不能算远端 CI 通过。实际远端注释保存在 [remote-ci-71a4bb5.json](../conformance/reports/remote-ci-71a4bb5.json)；旧提交记录保留，本地验证单独报告。

运行方式、固定上游 SHA、许可证与重放限制见 [conformance](../conformance/README.md)、[架构](ARCHITECTURE.md)、[来源](UPSTREAM.md) 和各包 README。
