# 实现与验收状态

更新日期：2026-10-21。当前实现以独立 Go package 实现公开的 Brief → ContextIR → H3 流程，运行时仅使用 Go 标准库，不依赖 Python、CLW、数据库或队列。它不包含 MiniMax 托管 Context-IR 的内部代码，也没有托管服务或成片质量等价证据。

## 当前验收结论

固定上游 `fd031e136ae3d89147324c0d1b2aa65e838c21f5` 的原清单有 **42 文件、991 个条目**。验收单位是用例保护的业务规则，允许使用 Go 的合法输入、显式观察依赖和真实命令，不机械复制 Python 实现细节。本轮所有条目都有已执行且通过的映射：672 项原断言验证 Go 主输出，319 项独立 Go 适配，N/A 为 0。新增契约变更版本门卫、公共 Compile 镜头约束、最终媒体绑定/姿态和合法双图 FL2VA 对照。完整结果见 [本轮逐项报告](../conformance/reports/native-functionality-intent-review-v1.json) 与 [原意及补强说明](UPSTREAM_TESTS.md#go-测试怎样保留上游意图)。

| 证据类型 | 项数 | 实际验证的内容 |
| --- | ---: | --- |
| 未修改的原断言验证 Go 主输出 | 672 | 每个原 nodeid 的 call phase 通过，要求的主 Go operation/validator 确实执行；不可变 bridge、当前 runtime、测试适配器、fixture、录音请求来源均核验 |
| 独立 Go 接口/输入适配 | 319 | 原要求对应实际 Go API、成品和错误负例；明确记录合法模式、依赖注入、结构化输入或版本差异，不以辅助函数触达代替主行为 |
| 免除 / N/A | 0 | 原 Python 语法测试的源码解析/声明最低版本可用要求，按 Go 兼容性契约实际验证 |

319 项包括原先 126 个 Python-only nodeid 的全部 126 个要求，以及 193 个此前虽然调用过 Go、但主行为或接口尚需独立验证的节点。完整映射没有遗漏或重复。普通 `go test`、原断言桥接和付费模型运行各自保存原始结果，不把它们的通过数相加。

未修改 Python 断言的桥接诊断仍是 **981 passed / 10 failed，0 skipped**，见 [原报告](../conformance/reports/go-draft-pytest-go-compatibility-v1.json)。这些输入、依赖、版本及命令差异不能直接当作 Go 业务失败，也不能直接豁免原功能；应检查保持原意的 Go 正向产物和负向对照。十项均有明确映射，原结果没有被改写。逐项说明见 [UPSTREAM_TESTS](UPSTREAM_TESTS.md#原样重跑保留的十项差异)。旧失败轮次继续保留。

本轮全仓 Go 测试、race、vet、格式与 diff 检查以 [本轮质量记录](../conformance/reports/local-quality-intent-review-v1.json) 为准，真实最低 Go 1.25.0 全包测试和 vet 见 [本轮最低版本记录](../conformance/reports/go-minimum-intent-review-v1.json)。旧报告保持不变；新增补强不再计作新的上游条目，既有真实高层编译录音继续严格回放。

## Go 源码与最低版本兼容性

原两项 Python 语法测试的原意已按 Go 实现，不再列 N/A：`TestGoCompatibilityEverySourceParses` 解析实际 Go 源码，`TestGoCompatibilityDeclaredLanguageVersion` 检查 `go.mod` 声明版本对应的语言与标准库 API。负例包括当前编译器能读、较低语言版本拒绝的语法，以及较新标准库 API；原 Python f-string 规则没有伪称逐句翻译为 Go。

更强的 [真实最低 Go 1.25.0 全包测试与 vet](../conformance/reports/go-minimum-intent-review-v1.json) 已通过，关闭自动升级与工作区，使用新缓存且未读取模型凭据。聚合门卫核对真实工具链、所有 Go/test/embedded 输入 hash、完整 JSON 事件、两项实际测试和运行期间证据不变；用 Go 1.27 或 Go 1.25.1 代替最低版本、筛选测试、跳过、篡改来源或日志均被拒绝。映射见 [remaining_go_compatibility.json](../conformance/adaptations/remaining_go_compatibility.json)。旧 989 项通过 / 2 项 N/A 的 [历史报告](../conformance/reports/native-functionality-remaining-all-v3.json) 保持原样。

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

本轮许可录音重新运行当前合同：主 437 entries / 228 unique 中 1 unique 失败（14 entries 复用该失败）；同合同显式重试这 14 entries / 1 unique 成功，另补录 10 unique。完整索引保留 **461 记录、238 个完整输入**和初轮失败，禁止旧补充录音覆盖新索引。原断言重放的 423 次许可/风格调用在 Go 中验证全部 **1,422 次 Stage/System/Input/Schema**，其中一次真实非法 JSON 响应及后续重试也按原错误、用量和请求重放，没有丢掉失败调用。

此外有 [当前高层 Compile 三例独立核验](../conformance/reports/remaining-highlevel-compile-v2-verification.json)：显式/类型化选择转写的两例，以及按来源组合属性的身份合并一例。它们与 [原六例 suite](../conformance/reports/live-suite-round8/report.json)、[16 角色额外全 Compile](../conformance/reports/live-roles-round4/report.json) 和后续单例修复分别记录。旧轮次存在真实语义失败、非法 JSON 和拒绝，不能被新一轮 compiled 覆盖。当前三例也仍有警告，不能称全面生成质量通过。

六例 suite 使用原 PNG 真实像素；角色批次使用原测试的合成 cards，不能当成真实视频/音频观察。角色批次的生产 Compile 未携带原 deterministic_draft 单独的 mode 参数，因此不是同一函数输入合同。`semantic_review` 及人工审查能发现特定错误，但不是任意自由文本语义的完备证明。更没有执行真实 H3 视频推理，无法据 IR 测试证明成片人物一致性、动作、音画同步或文字保真。

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

当前产品提交 `84c82a35fe9f84b9179522f3925ff1d1d7e15324`，安装版本 `v0.1.1-0.20261021024005-84c82a35fe9f`，已从 GitHub 私有远端真实下载 9,385,184 字节 ZIP。独立消费者 build/run、IR/H3/哈希/评分重放通过，见 [消费者报告](../conformance/reports/private-consumer-go-compatibility-v4.json)。

下载包内执行完整 Go 测试：**372 主测试、7813 子测试全部通过，0 失败、0 跳过**，包含本轮新增的两个 Go 原生兼容性检查。源码与 ZIP 逐文件一致，下载源码 hash 也与真实 Go 1.25.0 最低版本全库验证一致；无 `replace`、工作区源码、模型凭据或 `conformance/reports` 依赖。见 [实际下载包测试](../conformance/reports/private-native-go-compatibility-v1.json) 与 [最终汇总](../conformance/reports/completion-go-compatibility-v1.json)。

三次下载安装因 GitHub TLS 连接中断失败，原记录全部保留。只对任务专用外部 Go VCS 缓存采用同一 GitHub 仓库的规范 `.git` URL 后，远端下载与全包验证通过；没有以工作树、overlay 或本地 replace 替代远端源码。此前 `874c5ee` 的分发证据继续保留。

首轮外部测试在 `dc2731a` 发现一项测试错误依赖 checkout 的 Git HEAD，记录保留在 [原失败报告](../conformance/reports/private-native-remaining-final-v2.json)。现按 Go 契约验证调用方传入的短/完整 commit、缺省未知状态和完整持久化，不要求模块目录存在 Git；修正后重跑逐项验收、全仓 race/vet 与上述外部全套。此前 `fe1feb7` 的旧分发证据和旧标签继续保留。

GitHub Actions 曾因账户付款/spending limit 在启动前被阻止，不能声称历史远端 CI 通过；[实际远端注释](../conformance/reports/remote-ci-71a4bb5.json) 和旧记录独立保留。当前以本地可复验的命令、原断言、独立 Go 测试和来源证明为依据。

更多内容见 [上游验收](UPSTREAM_TESTS.md)、[复验流程](../conformance/README.md)、[架构](ARCHITECTURE.md) 与各包 README。
