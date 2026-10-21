# 实现与验收状态

更新日期：2026-10-21。版本标识：`v0.2.0`，实际标签安装收据见 main 的发布索引。当前实现以独立 Go package 实现公开的 Brief → ContextIR → H3 流程，运行时仅使用 Go 标准库，不依赖 Python、CLW、数据库或队列。它不包含 MiniMax 托管 Context-IR 的内部代码，也没有托管服务或成片质量等价证据。

## 当前验收结论

固定上游 `fd031e136ae3d89147324c0d1b2aa65e838c21f5` 的原清单是 **42 文件、991 个条目**。当前 [逐项原生验收](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/native-functionality-release-v2.json) 为 **991/991，通过且无遗漏、重复或测试跳过**：672 项未修改原断言验证真实 Go 主输出，319 项使用按原意编写并执行的 Go 适配，N/A 为 0。条目不是 991 种独立能力，也不与 Go 测试事件或付费模型调用数相加。

| 证据类型 | 数量 | 实际验证的内容 |
| --- | ---: | --- |
| 原断言验证 Go 主输出 | 672 | 每个原 nodeid 的 call phase 和所需主 Go operation/validator；当前不可变 bridge、runtime、适配器、fixture 和完整录音请求来源 |
| 独立 Go 接口/输入适配 | 319 | 原业务规则的实际 Go API/产物及负例；显式记录合法模式、观察依赖、结构化输入与版本差异 |
| 免除 / N/A | 0 | 两项 Python 语法要求改为实际 Go 源码解析及声明最低版本兼容验证 |

319 项包括原先 126 个 Python-only nodeid 的全部要求及另 193 个原本虽触达 Go、主行为仍需独立验收的节点。完整分账和原意见 [UPSTREAM_TESTS](UPSTREAM_TESTS.md)。

[当前原样断言诊断 v8](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/go-draft-pytest-release-final-primary-v8.json) 仍为 **981 passed / 10 failed，0 skipped**；[来源核验](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-final-primary-v8-verification.json) 确认上述 672 项主输出通过。这十项输入、观察、profile、分类或接口差异没有被改成 Python 通过，各自另有保持原意的 Go 正反例。历史 v4 的 978/13、670/672 及所有旧失败继续保留。

当前 [全仓质量检查](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/local-quality-release-v1.json) 的 race、vet、源码 gofmt/diff 全通过：**442 主测试、7957 子测试，0 失败、0 跳过**。[真实最低 Go 1.25.0](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/go-minimum-release-v1.json) 全包测试及 vet 也通过。逐项聚合使用同一当前源码及 fresh v8 证据，核验运行期间源码不变。

原始 pytest 诊断保留其尾随空格；源码和测试的 diff 检查通过，完整未过滤检查中的原始日志格式记录见 [说明](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-source-diff-check-v1.json)。

本次确有产品修复：对白跨镜协议、最终叙事语义审查、身份/素材责任边界、参考板身份清理、源图映射及整数毫秒校验均有调整；不能描述成仅增加测试。

## Go 源码与最低版本兼容性

原两项 Python 语法测试的原意已按 Go 实现，不再列 N/A：`TestGoCompatibilityEverySourceParses` 解析实际 Go 源码，`TestGoCompatibilityDeclaredLanguageVersion` 检查 `go.mod` 声明版本对应的语言与标准库 API。负例包括当前编译器能读、较低语言版本拒绝的语法，以及较新标准库 API；原 Python f-string 规则没有伪称逐句翻译为 Go。

更强的 [真实最低 Go 1.25.0 全包测试与 vet](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/go-minimum-release-v1.json) 已通过，关闭自动升级与工作区，使用新缓存且未读取模型凭据。聚合门卫核对真实工具链、所有 Go/test/embedded 输入 hash、完整 JSON 事件、两项实际测试和运行期间证据不变；用 Go 1.27 或 Go 1.25.1 代替最低版本、筛选测试、跳过、篡改来源或日志均被拒绝。映射见 [remaining_go_compatibility.json](../conformance/adaptations/remaining_go_compatibility.json)。旧 989 项通过 / 2 项 N/A 的 [历史报告](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/native-functionality-remaining-all-v3.json) 保持原样。

## 实际提供的能力

| 能力 | 生产实现 |
| --- | --- |
| 高层编译与修订 | `Compiler.Compile` / `Refine` / 独立显式 `Draft`；模式、职责、替换目标、内容许可、参考优先级、镜头结构、受约束写作与有限修复 |
| 模式与素材职责 | 五种模式及上游全部 16 个角色；身份/姿态/参考图排版分离；源视频编辑/续写、环境、结构/分镜、主体放入/替换；目标不唯一时不猜测 |
| 用户固定内容 | 原文对白、旁白、歌唱、跨镜分片字节拼接、可见文字、音轨 cue；宿主回填固定槽位；真实时间边界与双端连续标记校验 |
| 音频与身份组合 | 音轨复制与属性参考分离；BGM `AudioCopyModes`；转写原文 `TranscriptUses` 的明确/类型化选择；`SubjectProjections` 按真实来源与选定属性合并身份 |
| 用户控制 | 固定镜头数/切点、摄影机闭集、四档创意、七个内置导演/自定义说明、LoRA 注册与权重/checkpoint/强度/触发词约束 |
| 唯一成品 | 权威 `Document`；同一 IR 派生 prompt、manifest、源元数据、准备指令、预算与 SHA-256；保存/读取/再编译 |
| 校验与预算 | 原 H3 规则/等级、模式格式、帧网格、画布和 latent rows、完整 Qwen2.5 精确 BPE、离线 Score/Aggregate/Compare/baseline gate |
| 真实素材与后端 | 文件头事实、ffprobe/ffmpeg 量测/抽帧、显式缓存、真实像素输入、可注入 ASR；`deepseek` / `chatcompat` 的显式模型/端点/结构化模式与逐次用量 |
| 宿主接口 | contract v3 标准库 HTTP handler、上传配额/在途预留/TTL/LRU、待澄清/版本记录、修订前依赖检查、公开 CLI、Comfy 图工具与远端 HTTP |

转写存在只表示词语证据，不能直接授权说话，也不表示听懂音色、音乐或节奏。所选词语须保持原语言和精确摘录，只进入固定对白，不能回显成画面文字。身份合并只保留明确选择的来源与属性，原观察和用户请求的目标属性各自保留来源。

`Refine` 使用原输入和观察重新编译；修改尺寸/时长时重算派生准备指令，不把旧派生值当作用户硬约束，也不修改原资产切片。未改变的约束、固定内容与执行选择保留。seed/effort 只有实际适配器明确支持时才可执行。

生成稿的协议/格式 helper 是显式选择的 API，返回可观察的修改记录；不会自动改写权威对白、可见文字或字面 RichText。非法模型输出通过 schema/协议边界作有限修复，不能用截短、猜标签或隐藏降级求通过。

## 真实模型证据与限制

最新统一批次是 [release-live-round6](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6/report.json)，产品运行时冻结在 `a9f84ef09bc0c662bd8b0c52afac380a1ce10dc4`，`composition-review/v10`，执行文件 SHA-256 为 `2b34869092e0110606f3a071c18af4f6183e565e4237b8612d28c455d91b7bfa`。生产文件已与实际安装的候选包逐项核对；最终标签的安装后收据单独记录。

批次完整执行 **35 Compile + 5 Refine = 40 操作**：36 个 compiled，4 个预期拒绝（3 个非法 Compile 输入、1 个删除对白来源但保留依赖原词的 Refine）。[独立产物检查](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-verification.json) 为 40/40；另有 [核心 21](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-semantic-core-review.json)、[角色 16](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-semantic-role-review.json)、[边界 3](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-semantic-boundary-review.json) 的逐项独立语义复核。reviewer 为 `coding_agent_independent_artifact_review`，不是人类签字或任意语义完备证明。

实际使用 `deepseek-flash`、`low`、输出上限 16384、并行 3；原 34 个 Compile 用 OfficialProfile 和显式 3/3 修复预算，新 wardrobe 原例明确用 ComfyUIProfile、1/1。完整记录共 **277 次调用，921,356 input tokens、538,309 output tokens**。有 14 次后续尝试：12 次 host structured validation、1 次 provider protocol、1 次 semantic review。其中 3 次 Refine 后续尝试属于最终预期拒绝，不能称作修复成功，也不能把 Result.Calls 重复 ledger 另加成调用。详见 [调用与失败清单](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-recording-inventory.json)。

本轮修复并验证了：固定中文跨镜片段/连续旗标及不可见旁白；明确对白/可见字的镜头位置，防止事件列表正确但后镜 prose 又表演/展示；voice 属性只作用于 vocal recipient，不能转用到发动机 SFX；replacement 动作继承；参考图角色和来源投影边界；非 actual-frame reference sheet 的单责身份资格阶段及最终 canonical 审查；T12 用整数毫秒比较，1200ms 合法而 1199ms 仍拒绝。源姿态、版页标签和镜头包装不会因此成为目标身份，实际首尾帧则保留整图像素责任。

prefix 案例的调用方原属性名 `wheels from the second picture` 是 opaque selector。加前置来源后，最终明确分源 clause/typed graph 仍把 wheels 绑定到实际 asset-2 / Picture3；删除前置来源后重排为 Picture2，物理来源不变。原属性名在 prose 中重复会产生位置命名歧义，本报告明确保留，不能说所有散文均无歧义，更不能据此声称成片来源已证实。库不按词法规则重解释该字符串。

许可索引 [release_final_licence_replay_resolutions_v2.json](../conformance/fixtures/release_final_licence_replay_resolutions_v2.json) 保留原 461 entries，追加 2 个原 DecideLicence API 补录，共 463 entries / 238 个完整输入。两例补录属于 [独立 v9 helper 批次](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-helper-refresh-v1/report.json)，8 次调用、上限 12288、binary `ff97523c…`；它与本轮 v10 main 的 `2b348690…` 不是同一 binary，也不计入 main 的 277 次。每条录音按自身 binary/来源核验完整 Stage/System/Input/Schema，不能将历史回放称作当前新付费调用。

本轮三例高层 Compile 和 wardrobe 原例的完整录音已导入自包含 [highlevel](../compiler/testdata/remaining_highlevel/manifest.json) / [wardrobe](../compiler/testdata/remaining_wardrobe_fresh/manifest.json)，逐例实际阶段数为 6、6、10、12。它们是上述 main 调用的子集，回放不是新的 34 次调用；原失败 critic、修复、用量和完整 normalized Result 均保留。

真实 PNG 感知用于原 ref2 双主体、i2 动画和 paired 首尾帧三个 Compile 场景。其他音频/视频及多数图像 Card 是明确 supplied typed observations，不能声称真实听懂或观察了音视频。转写词的授权重演与原音轨 signal copy 分开；语音属性本身不自动授权台词。未执行实际 H3 视频生成，无法据 IR 测试证明成片身份、动作、音画同步或文字保真。

所有旧模型失败、非法 JSON、拒绝、WARN 和 [oracle 更正](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round5-visible-text-oracle-correction.json) 保留。本轮 checker 首次因局部变量 NameError 中止，旧源码与异常也保留在 [工具纠正记录](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/release-live-round6-checker-tool-correction.json)；之后用实际修正脚本得到 40/40，不伪称旧脚本执行通过。

## 明确的接口与执行边界

- 意图和权限通过类型化模型合同解释，不移植关键词意图分类器；未知、矛盾或缺少来源时明确失败。
- 未解决的机制选择默认 `needs_input`；继续发布必须明确选择 `ClarificationContinue` 并保留原问题。
- 已提供内容与禁止要求冲突默认拒绝；只有显式 `ScopeConflictSuppliedWins` 采用源项目策略并记录冲突。结构化全静音仍不可覆盖。
- `Silent` 禁止新增配乐；已提供 BGM 的复制、全面禁止音乐、全面禁止声音分别建模。完整复制最终音轨不能再暗中混入新内容。
- 默认 `FramePolicyExact`；显式 `FramePolicySnapUp` 才按源网格对齐并报告名义/实际时长。遗漏 seed 保持未指定。
- 实际首尾帧使用 `first_frame` / `last_frame`；不复制源旧 manifest 的错误 socket。`OfficialProfile` 与 `ComfyUIProfile` 能力不同，不通过改模式或丢锚点适配。
- `MediaPreparation` 表示所需裁剪/缩放/帧数准备，不声称已执行。`comfy` 不自动完成素材变换、manifest socket 注入或 LoRA loader 配置；实际准备和视频执行由宿主负责。
- 部分视频采样覆盖不能冒充完整观察；音频元数据/ASR 不能替代音色或音乐观察。默认覆盖校验不放宽。
- 默认编译失败不返回草稿；`Draft` 和失败后草稿策略均须显式选择，保留原因和实际用量。

## 发布与外部验证

候选 `38393570e2c87365c26be278d0550dae3cd389b7` 已从私有远端实际安装，ZIP 为 10,835,560 字节；下载包 442 主测试、7957 子测试通过，完整 40 操作/277 调用尝试严格回放通过。候选安装和重放证明保存在审计分支；最终标签证明以 [发布索引](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json) 为准。

完整审计已保留在独立分支，发行树保留 CI 的 4 个原基线及 8 个完整录音目录，共 37,030,618 字节；原有逐字节来源核验与 CI 逻辑不变。见 [保留清单](../conformance/reports/release-thin-ci-keep-set-v1.json)。生产 Go、testdata 和原 fixtures 均未修改；完整付费录音在审计 checkout 中提供。

最终标签的具体提交、OriginHash、module ZIP 与安装后全测/40 项重放收据见 [main 发布索引](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json)。候选通过不代替最终标签的实际安装验证；索引逐项记录其执行状态。

以下是保留的历史分发证明，不能冒充本轮 `v0.2.0`：产品提交 `84c82a35fe9f84b9179522f3925ff1d1d7e15324`、版本 `v0.1.1-0.20261021024005-84c82a35fe9f` 曾从 GitHub 私有远端下载 9,385,184 字节 ZIP。[消费者报告](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/private-consumer-go-compatibility-v4.json)、[下载包原生测试](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/private-native-go-compatibility-v1.json) 和 [当轮汇总](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/completion-go-compatibility-v1.json) 记录 372 主测试、7813 子测试通过，无失败/跳过，无 replace、工作树或 reports 依赖。三次 TLS 下载失败及同仓库规范 `.git` URL 处理均保留。

更早 `dc2731a` 外部测试依赖 checkout Git HEAD 的 [真实失败](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/private-native-remaining-final-v2.json)、修正后显式 caller provenance/未知状态测试，以及 `874c5ee`、`fe1feb7` 的历史分发记录均保留。旧标签和失败不被覆盖。

GitHub Actions 曾因账户付款/spending limit 在启动前被阻止，不能声称这些历史远端 CI 通过；[远端注释](https://github.com/ly95/contextir/blob/b8f24802ac310089eabf7918d5beeb5721d58d62/conformance/reports/remote-ci-71a4bb5.json) 独立保留。当前以实际本地、原断言、Go 测试及来源证明为依据。

更多内容见 [上游验收](UPSTREAM_TESTS.md)、[复验流程](../conformance/README.md)、[架构](ARCHITECTURE.md) 与各包 README。
