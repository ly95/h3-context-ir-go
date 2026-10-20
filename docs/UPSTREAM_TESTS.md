# 上游测试基线与 Go 验收

目标是复用 `ruashots/open-h3-ir` 的完整行为测试与现有样例，逐项验证 Go 实现。自编的三个 DeepSeek 案例没有替代这里的任何测试。

固定来源：[`fd031e136ae3d89147324c0d1b2aa65e838c21f5`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)。该版本 Python 包版本为 0.4.1。所有导出都拒绝不同 SHA 或已修改的上游源码。

## 目前真实执行了什么

- 原样 Python 全套 pytest：42 个测试文件，991 个唯一 nodeid；991 passed，0 failed，0 skipped。环境只有一个 Starlette 弃用警告。
- 原样 Python controls：23/23。包含官方 Ref2VA 示例及明确的 P5 例外、16 个有指定规则的缺陷变体、一个应当机械合法的审美反例，以及四个模式的发布样例。
- 原样 Python contract falsification：17/17 planted defects 被 guard 检出（RED），0 GREEN，0 BROKEN；在独立临时副本执行，修改后源码 SHA-256 全部恢复。上游文档仍写 13，但固定提交的脚本实际有 17 项，以脚本实际结果为准。
- pytest 原断言运行时导出 386 次 validator 调用（来自 295 个测试）和 1,952 次 planner/mode/manifest 调用，保留输入、原始上下文、输出和所属 nodeid。
- 导出上游六个 eval Brief 和原始资产。它们没有自动成为 Go 已通过的 live eval，`live_status` 初始为 `not_run`。

**以上 Python 通过数不计入 Go 通过数。** `conformance/matrix.json` 为每个 nodeid 保存参数、assert 原文及行号、setup/call/teardown 执行结果。Go 初始状态都是 `pending`，必须由实际 Go 差分或桥接执行的报告更新。

## 原生 Go 执行的边界

`h3check` corpus 差分把同一份 text 和 Context 交给 Go，比较完整规则 ID 和 severity 的多重集合。它验证导出输入上的检测行为；单独比较这两个字段不能证明修复消息或整条编译流程正确。

`pytest_go_validate.py` 把上游 pytest 中的 `validate()` 调用替换成纯 Go JSONL 进程，再执行未经修改的原始断言，包括错误消息和修复建议。报告逐 nodeid 标记是否实际触达 Go。未触达 Go 的测试仍运行 Python，必须记为 `not_covered`。即使这个混合运行显示 991 passed，也只能声称 Go validator 触达的 295 个测试通过，不能声称整个 Go 编译器通过 991 个测试。

最新 core 原断言桥接 round19 实际运行 991 项全部通过；770 个 nodeid 实际触达 Go，221 个仍只运行 Python。Go 部分包含 validator、grid、token、完整 plan.Build、LoRA、Comfy 图、真实媒体 header/probe/frame sampling/cache、上传存储、Chat Completions 后端、HTTP intake/handlers、合同 JSON/JS、director/Scope helpers，以及三个 h3 style/summary 纯格式函数。原 38 个上传端点、31 个上传存储、6 个服务错误测试都实际触达 Go 并通过。各历史失败报告保留，未修改原断言；不同桥接运行的通过数不能相加成为整条流程通过数。完整逐文件覆盖见下表。

另有隔离的完整 plan 桥接 round3：原样 991 项全部通过，312 次原 `build_plan` 调用实际执行 Go `plan.Build`；731 个 nodeid 触达某个 Go 组件，260 个仅 Python。该运行的 mode 语义由原 Python 决定，不能用它覆盖真实模型 mode 的失败。发布路径仍拒绝未绑定的替换目标；用于原内部规划断言的 inspection API 显式保留 unresolved 元数据。

素材许可与风格也已经独立捕获完整原样运行：991 个测试仍通过，导出 1,617 次真实函数调用。每项保留原 Brief/cards、许可归属、变换片段、风格结果、属性及原断言，用于独立验证生产模型的 typed 语义输出。导出的 Python 结果不能作为模型输入。

第六轮真实 DeepSeek 语义请求与 held-out expected 严格分开。两个先前人物错绑已由新的真实输出修正，完整 plan corpus 为 1,927 项 JSON 完全一致、21 项仅 mode 值一致，以及 4 项符合原测试最终 intake 拒绝。新冻结的 ca28 构建完成真实 mode 运行后，当前 Go 重放已通过全部原 mode 断言；此前错误原文冲突审计、大小写协议和一次 “animate this photo” 误选均有失败记录。当前 21 个 mode 值、精确信心及规则匹配，4 个最终 intake 拒绝独立判断正确；独立 held-out 元数据仍有 15 项 alternatives 与源不同，因为 Go 按实际 asset 数量和 profile 过滤可执行备选，不把源代码给出的不可执行选项当作兼容目标。这些差异单列，没有伪称逐字段全等。mode 值一致不能替代完整行为验收；第一至四轮原请求、输出与失败均保留。

第四轮完整素材许可真实运行覆盖 437 原调用（228 唯一输入），使用可重建的 ca28 冻结构建。独立字段检查为 395 项吻合、41 项有差异、1 项决策失败。更强的原断言回放得到 986 passed、5 failed：199 个 nodeid 使用历史 Go 结果（194 passed、5 failed），792 个仍是 Python。失败分别是 kept-medium 提示文字、high-contrast grade 的 palette 归属、一个 identity 合同失败、spoken 动作归属，以及原旧词表无法分类的 1990s cel animation 在 native 被分类为 2D 的差异。各历史运行保留；字段全等和原 assert 是两种独立证据，不能相互替代。结果回放不代表当前 binary 的整条原生编译流程已通过。

创意 scope 另从原测试捕获 435 次调用，其中 125 次 build、48 唯一模型输入。真实模型结果经过当前生产 Go `DecideScopeInput` 后，原 991 项运行得到 990 passed、1 failed；125 次 build 均执行 Go。原测试要求 supplied dialogue 覆盖陈旧 no-dialogue，当前 Go 则保留显式冲突要求调用方解决，这是尚未达到原语义等价的一项差异。独立 held-out permission 检查为 124/125 吻合。Scope 的 note/brief_instruction/WhyNot 与集合代数随后已移植并接生产 Go，scope round2 原断言仍只有同一默认冲突差异。显式选择公开 `supplied_content_wins` 策略的独立 scope-legacy round1 为 991 passed（750 个 nodeid 触 Go、125 次 build）；冲突与解决策略同时记录，不覆盖默认 reject 的失败证据。

六个原 suite Brief 第五轮为 6 compiled；第四轮是 5 compiled、1 failed，第三轮是 5 compiled、1 needs_input，第二轮是 5 compiled、1 failed，第一轮是 1 compiled、5 failed。各轮原始 trace 均保留。16 个原 ROLE_CASES 也独立走完整生产 Compile，输入为原 Brief 和调用者提供的合成 cards，未冒充实际媒体观察；第一轮 14 compiled、2 failed。独立结构字段检查另列，人工语义审查仍发现场景泄漏、音频 copy/property 关系和正文丢失角色等问题。compiled 仅说明记录的原生编译和机械校验通过；needs_input 表示明确请求澄清。未进行真实 H3 视频渲染，也没有据此宣称文字、素材理解和视频质量合格。

原 renderer/textnorm 另捕获 1,927 次真实调用及 mutation 后状态，其中三个纯 style/summary 格式函数的 250 次原调用与 Go 字节精确一致，并接入权威 Document→H3 生产 renderer。其余原 `render_ir` 与原 Python orchestration 尚未由原断言桥替换，不能根据三个 helper 通过宣称全 renderer 通过。

同时接 mode/scope/licence 的组合原断言运行已建立。ca28 源码快照的 222 个文件逐一核对 hash 后，在独立目录构建对应 Go bridge，再重放真实录音，得到 977 passed、14 failed：857 个 nodeid 实际触 Go（843 passed、14 failed），134 个仍 Python。14 项包括上述 5 项 licence 差异，以及 9 项 native audio DTO 改变后没有完全相同真实录音的桥接缺口。后者保留实际 analyzer provenance，单独导出九条真实 native 输入；随后已在新 972a 构建得到九条实际决策，等待同批完整录音统一回放，没有删除字段或归一化输入冒充旧录音。第一轮旧 schema 的 200 项失败和一次 working-checkout 的 18 项失败也原样保留。离线 CI 强制原 991 项全过、零 skip 和 native 覆盖不缩水，当前仍如实失败。

执行证据并集为 857/991 个 nodeid 触达 Go（包括上述失败的组合调用），134 个仍仅 Python。并集不等于通过数，当前逐条状态见 `coverage_union.json`。其中两条纯 Python 语法检查明确不适用于 Go；其余 132 项分别标出已有 native API 但未接原断言、旧 regex API 的 typed 替代、未验证的 pipeline/helper/metric、独立 fixture oracle 等，不把所有缺口都简化成“只是桥没接”。完整清单在 `coverage_gaps.json`。

显式 `media.AssetCardCache` 已接生产 image/video 感知，键包括真实内容、完整 prompt/schema/input、模型策略和分析版本；音频 metadata 永不缓存，video cache 不复用每次的 ASR。原 5 个缓存测试执行 Go 并通过。真实图片/AV 缓存、失效、破损、音频污染与 usage 不重放的 native race 测试也通过。

完整库能力还要在测试矩阵中覆盖结构规划、所有角色和模式、帧网格与容量、输入拒绝、素材许可与覆盖关系、跨镜对白、创意等级和导演提示、感知证据、模型失败/修复语义、传输合同、上传与存储等行为。独立 Go API 可以采用不同代码结构；等价声明必须指向真实运行的输入、断言与 Go 产物。

## 文件与重跑

运行方式和夹具协议见 [`conformance/README.md`](../conformance/README.md)。原始 fixture 与许可证保存在 `third_party/open-h3-ir/`。`source_manifest.json` 记录上游所有 tracked 文件以及复制文件的 SHA-256，golden 文本和两个 PNG 复制时不改一个字节。

下表单列 core round19，最新组合运行的逐文件通过、失败及纯 Python 数另外保存在 `coverage_by_file.json`，逐 nodeid 保存在 `matrix.json` 的 `go_combined_assertions`。

| 上游文件 | 原测试 | Python 基线通过 | Core Go 组件通过 | Core Go 组件失败 | Core 仅 Python | 整体原生行为 |
|---|---:|---:|---:|---:|---:|---|
| `tests/test_anchor_downgrade_is_honest.py` | 5 | 5 | 3 | 0 | 2 | pending |
| `tests/test_ask_states_the_wiring.py` | 10 | 10 | 10 | 0 | 0 | pending |
| `tests/test_audio_card_is_not_cached.py` | 5 | 5 | 5 | 0 | 0 | pending |
| `tests/test_audio_claims.py` | 19 | 19 | 16 | 0 | 3 | pending |
| `tests/test_capacity.py` | 13 | 13 | 13 | 0 | 0 | pending |
| `tests/test_closed_vocabulary.py` | 17 | 17 | 16 | 0 | 1 | pending |
| `tests/test_contract.py` | 22 | 22 | 15 | 0 | 7 | pending |
| `tests/test_creativity.py` | 57 | 57 | 37 | 0 | 20 | pending |
| `tests/test_declared_roles.py` | 21 | 21 | 20 | 0 | 1 | pending |
| `tests/test_deterministic.py` | 58 | 58 | 52 | 0 | 6 | pending |
| `tests/test_dialogue_across_a_cut.py` | 25 | 25 | 25 | 0 | 0 | pending |
| `tests/test_director.py` | 38 | 38 | 16 | 0 | 22 | pending |
| `tests/test_endpoint_portability.py` | 43 | 43 | 42 | 0 | 1 | pending |
| `tests/test_enrichment_path_imports.py` | 4 | 4 | 4 | 0 | 0 | pending |
| `tests/test_every_role_renders.py` | 40 | 40 | 39 | 0 | 1 | pending |
| `tests/test_ffmpeg_absent.py` | 3 | 3 | 3 | 0 | 0 | pending |
| `tests/test_file_facts.py` | 26 | 26 | 26 | 0 | 0 | pending |
| `tests/test_fix_loop.py` | 24 | 24 | 21 | 0 | 3 | pending |
| `tests/test_frame_anchors.py` | 19 | 19 | 18 | 0 | 1 | pending |
| `tests/test_golden_asset_geometry.py` | 2 | 2 | 0 | 0 | 2 | pending |
| `tests/test_hardening.py` | 49 | 49 | 47 | 0 | 2 | pending |
| `tests/test_instruction_line.py` | 19 | 19 | 19 | 0 | 0 | pending |
| `tests/test_megapixels.py` | 6 | 6 | 6 | 0 | 0 | pending |
| `tests/test_music_referenced_not_copied.py` | 37 | 37 | 30 | 0 | 7 | pending |
| `tests/test_paired_soundtrack.py` | 8 | 8 | 8 | 0 | 0 | pending |
| `tests/test_pinned_shots.py` | 17 | 17 | 12 | 0 | 5 | pending |
| `tests/test_reference_audio_words.py` | 13 | 13 | 13 | 0 | 0 | pending |
| `tests/test_reference_licence.py` | 154 | 154 | 52 | 0 | 102 | pending |
| `tests/test_request_validation.py` | 23 | 23 | 23 | 0 | 0 | pending |
| `tests/test_retention_bookkeeping.py` | 17 | 17 | 17 | 0 | 0 | pending |
| `tests/test_retry_varies_the_question.py` | 6 | 6 | 6 | 0 | 0 | pending |
| `tests/test_score_and_length.py` | 11 | 11 | 11 | 0 | 0 | pending |
| `tests/test_service_errors.py` | 6 | 6 | 6 | 0 | 0 | pending |
| `tests/test_shot_planning.py` | 19 | 19 | 7 | 0 | 12 | pending |
| `tests/test_storyboard_role.py` | 5 | 5 | 3 | 0 | 2 | pending |
| `tests/test_structure_role.py` | 5 | 5 | 3 | 0 | 2 | pending |
| `tests/test_style_role.py` | 5 | 5 | 3 | 0 | 2 | pending |
| `tests/test_supported_python_versions.py` | 2 | 2 | 0 | 0 | 2 | pending |
| `tests/test_swap_roles.py` | 59 | 59 | 46 | 0 | 13 | pending |
| `tests/test_upload_endpoint.py` | 38 | 38 | 38 | 0 | 0 | pending |
| `tests/test_upload_store.py` | 31 | 31 | 31 | 0 | 0 | pending |
| `tests/test_video_reference.py` | 10 | 10 | 8 | 0 | 2 | pending |
