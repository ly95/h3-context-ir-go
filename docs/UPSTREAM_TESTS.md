# 上游完整行为验收

**尚未达到完整原生等价。最新同源冻结、完整 Go Draft 入口的原断言运行是 986 passed、5 failed：865 个 nodeid 实际执行 Go，其中 860 passed、5 failed；126 个仍仅执行 Python。** 原始 991 条断言与输入没有修改，0 skipped、0 xfailed。

本轮 117 个 nodeid 实际进入公开 `Compiler.Draft`，共 125 次调用，121 次返回原生 Document/Artifact；122 次渲染实际调用同一 Document 的 `h3.Compile`。另有 52 个 nodeid 只调用 Go `plan.Build`，696 个调用其他 Go 组件，不能称为完整 Draft 已通过。原 evaluate 的 8 个 nodeid / 14 次调用也在本轮实际执行 Go。

详见 [完整 JSON](../conformance/reports/go-draft-pytest-9454474408ea-frozen-dto-v2.json)、[原 pytest 输出](../conformance/reports/go-draft-pytest-9454474408ea-frozen-dto-v2.txt)、[逐节点边界](../conformance/reports/draft-boundaries-9454474408ea-frozen-dto-v2.json) 与 [5 项失败原值](../conformance/reports/draft-failures-9454474408ea-frozen-dto-v2.json)。本轮 bridge `5490448e922c…` 由产品 `9454474408ea…` 的 279 文件快照派生，唯一差异是测试桥的两项 DTO 投影，独立 archive 逐文件记录。产品 eval 二进制实际重建仍与原 `9454474408ea…` 字节相同；[来源证明](../conformance/reports/go-draft-pytest-9454474408ea-frozen-dto-v2-provenance.json) 同时归档 adapter 源码、fixture hashes 和构建记录。旧 953/38、971/20 等失败报告全部保留。

## 最新严格边界与剩余差异

原 `deterministic_draft(brief, mode, cards, ...)` 入口真实执行 Go Draft，mode 来自原调用参数；返回的 NativePlan 由原生 Artifact 渲染，Python writer 不生成其正文。独立许可/风格边界使用真实模型录音，显式选择部分 cards 策略，不声称观察完整媒体。其他 source Plan 的 renderer、部分 composer/orchestration 与静态 helper 仍是 Python。

以下 **5 个原 nodeid 仍然失败**，没有豁免或标绿：

| 原 nodeid | 实际 native 错误/行为 |
|---|---|
| `tests/test_director.py::test_direction_cannot_move_a_shot_count_the_caller_pinned` | `invalid_document` / `request` / `assets`：reference mode requires at least one media asset。原内部构造传 ref2va 与空 assets。 |
| `tests/test_hardening.py::test_every_label_in_the_text_has_a_manifest_entry_behind_it` | `perception_failed` / `perception` / `assets`：缺少 supplied observed evidence 或 perception adapter；caller note 没有伪作已观察。 |
| `tests/test_reference_licence.py::test_a_frame_anchor_DOES_licence_its_pose` | `unsupported_capability` / `compiler` / `bindings.anchor_ms`：当前 runtime 不能保证 reference 模式 exact frame anchor，明确拒绝。 |
| `tests/test_reference_licence.py::test_a_frame_anchor_is_exempt_from_a_transformation_marker` | `invalid_document` / `request` / `assets`：keyframe mode requires its exact number of image assets。原内部构造传 fl2va 与一张图。 |
| `tests/test_reference_licence.py::test_the_rule_is_NOT_armed_when_the_requested_medium_is_unclassifiable` | 原断言要求 `1990s cel animation` 的旧词表分类是 None，actual requested_medium 为 `2d-animation`；没有伪造 unknown。 |

另外两项源内部表示已由原断言实际验证：base SourcePlan 的观察元数据仅从真实 native Evidence 投影，与 target Document.Subjects/appearances 分开；`{{CAM}}` 仅映射已验证的实际 camera Event。packet 的 `dto_projections` 明示这些测试 DTO 政策，native Artifact 始终直接由原 Document 的 `h3.Compile` 产生。945 原 7-failure 轮保留，未追改。

423 次许可/风格调用消费的 **1,418 个实际模型请求**均由 Go 在消费录音前核对 Stage/System/Input/Schema；0 未验证消费、0 缺少 exact input、0 不适用录音。972a 全量结果与 448 环境/permission 补录的 binary 谱系分别保留，未把重放称作新付费调用。详见 [请求适用性报告](../conformance/reports/semantic-replay-applicability-9454474408ea-frozen-dto-v2.json)。

## 历史组件证据

| 运行 | 原断言结果 | 实际 Go 边界 |
|---|---:|---|
| [原样 Python 基线](../conformance/reports/upstream-pytest.json) | 991 passed | 0 个 Go；42 个测试文件，0 skipped |
| [validator 原断言](../conformance/reports/go-validator-pytest-round3.json) | 991 passed | 295 个 nodeid，其余 696 个 Python |
| [core round19](../conformance/reports/go-core-pytest-round19.json) | 991 passed | 770 个 nodeid，221 个 Python；原 Draft body/renderer 仍 Python |
| [完整 plan round3](../conformance/reports/go-plan-pytest-round3.json) | 991 passed | 312 次真实 plan.Build；mode 语义仍由 Python 决定 |
| [972a 同源组合及十条真实 native-audio 补录](../conformance/reports/go-combined-pytest-972a-supplement-complete.json) | 990 passed、1 failed | 857 个 nodeid；尚未替换完整 Draft，唯一失败为 cel 分类合同 |
| [许可历史产物原断言](../conformance/reports/go-licence-result-pytest-round5.json) | 990 passed、1 failed | 199 个 nodeid 消费已记录 Go 结果，其余 Python；不等于当前 binary 执行 |
| [显式 supplied_content_wins scope 策略](../conformance/reports/go-scope-legacy-pytest-round1.json) | 991 passed | 125 次 native scope build；默认 reject 的历史冲突失败另存 |

旧 schema 200 项失败、工作树组合的 18 项失败、ca28 同源组合的 14 项失败及其他历史轮次均保留在 `conformance/reports`。不把不同运行的通过数相加，也不把覆盖并集当作通过数。

## 真实模型全 Compile 回归

最新 08e0 冻结构建的 [六个原 suite Brief](../conformance/reports/live-suite-round8/report.json) 为 6 compiled；[十六个 ROLE_CASES Brief/cards 子集的额外全 Compile](../conformance/reports/live-roles-round4/report.json) 为 15 compiled、1 failed（environment）。请求、原始输出、拒绝文本、usage validity、模型及 binary SHA 全部归档。448 版本另有独立 [environment 单例补录](../conformance/reports/live-roles-environment-round1/report.json) 和 [animate 单例补录](../conformance/reports/live-suite-style-round1/report.json)，均 compiled；旧整批失败保留。suite runner 的 input_sha256 为空，单例归档明确不把空 hash 相等当作输入已验证。compiled 只证明记录的编译/机械路径成功，正文角色效果和媒体理解必须独立审查，未进行 H3 视频渲染。

ROLE_CASES 的全 Compile 批次复用原 Brief/cards，但**未携带原 deterministic_draft 的独立 mode 参数**，由生产 Compile 重新推断 mode，因此不是相同函数输入合同。原始 mode 和省略项由实际捕获参数记录在 [roles-compile-contract.json](../conformance/reports/roles-compile-contract.json)；不能只凭低层 expected mode 将这种跨合同差异判作错误。新的 wholeDraft 原入口已使用实际 mode 参数。

许可全量 437 调用/228 唯一输入在 972a 得到全部实际决策；独立字段检查 379 项相同、58 项有差异。环境 10 rows/3 唯一输入随后以 08e0 独立补录，native audio 另有 9+1 条实际补录，均未归一化或删掉 native provenance。448 又独立补录环境 10 rows/3 unique 和 permission 14 rows/1 unique，只有实际请求适用时才消费。跨版本重放必须精确匹配输入、保留各自产物 binary，并验证实际 Stage/System/Input/Schema 请求适用性；不能把旧录音说成新 binary 的模型调用。

## 固定来源与剩余覆盖

上游固定为 [`fd031e136ae3d89147324c0d1b2aa65e838c21f5`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)，Python 0.4.1。原 991 项、23 controls、17 planted-defect RED 均已原样运行；Python 通过不计 Go 通过。386 次 validator、1,952 次 plan/mode、1,617 次 licence/style、1,927 次 render/text 调用保留原参数、输出、assert 和 nodeid。模型输入与 held-out expected 独立。

当前 126 个纯 Python nodeid 的精确清单和功能/桥接缺口分类分别见 [coverage_union.json](../conformance/reports/coverage_union.json) 和 [coverage_gaps.json](../conformance/reports/coverage_gaps.json)。仅两条 Python 语法检查不适用于 Go；其他缺口没有豁免。表中触 Go 也可能只执行一个组件，整体行为仍未自动升级为已实现。

`source_manifest.json` 记录所有 upstream tracked 文件与复制资产 hash；11 个 golden/PNG 字节不变，保留 Apache-2.0 及 NOTICE。`reports/builds` 的后续源码快照可重建；历史纯 hash 清单不能恢复旧源码。原样测试直接从干净固定 checkout 执行。重跑命令、环境与协议见 [conformance/README.md](../conformance/README.md)。

## 最新 wholeDraft 原断言逐文件结果

| 上游文件 | 原测试 | Go 触达并通过 | Go 触达并失败 | 仅 Python | 完整 Draft 入口 nodeid |
|---|---:|---:|---:|---:|---:|
| `tests/test_anchor_downgrade_is_honest.py` | 5 | 3 | 0 | 2 | 2 |
| `tests/test_ask_states_the_wiring.py` | 10 | 10 | 0 | 0 | 10 |
| `tests/test_audio_card_is_not_cached.py` | 5 | 5 | 0 | 0 | 0 |
| `tests/test_audio_claims.py` | 19 | 16 | 0 | 3 | 0 |
| `tests/test_capacity.py` | 13 | 13 | 0 | 0 | 2 |
| `tests/test_closed_vocabulary.py` | 17 | 16 | 0 | 1 | 2 |
| `tests/test_contract.py` | 22 | 15 | 0 | 7 | 0 |
| `tests/test_creativity.py` | 57 | 52 | 0 | 5 | 1 |
| `tests/test_declared_roles.py` | 21 | 20 | 0 | 1 | 3 |
| `tests/test_deterministic.py` | 58 | 52 | 0 | 6 | 0 |
| `tests/test_dialogue_across_a_cut.py` | 25 | 25 | 0 | 0 | 2 |
| `tests/test_director.py` | 38 | 16 | 1 | 21 | 3 |
| `tests/test_endpoint_portability.py` | 43 | 42 | 0 | 1 | 0 |
| `tests/test_enrichment_path_imports.py` | 4 | 4 | 0 | 0 | 0 |
| `tests/test_every_role_renders.py` | 40 | 39 | 0 | 1 | 37 |
| `tests/test_ffmpeg_absent.py` | 3 | 3 | 0 | 0 | 0 |
| `tests/test_file_facts.py` | 26 | 26 | 0 | 0 | 0 |
| `tests/test_fix_loop.py` | 24 | 21 | 0 | 3 | 0 |
| `tests/test_frame_anchors.py` | 19 | 18 | 0 | 1 | 4 |
| `tests/test_golden_asset_geometry.py` | 2 | 0 | 0 | 2 | 0 |
| `tests/test_hardening.py` | 49 | 46 | 1 | 2 | 10 |
| `tests/test_instruction_line.py` | 19 | 19 | 0 | 0 | 4 |
| `tests/test_megapixels.py` | 6 | 6 | 0 | 0 | 0 |
| `tests/test_music_referenced_not_copied.py` | 37 | 30 | 0 | 7 | 10 |
| `tests/test_paired_soundtrack.py` | 8 | 8 | 0 | 0 | 0 |
| `tests/test_pinned_shots.py` | 17 | 12 | 0 | 5 | 0 |
| `tests/test_reference_audio_words.py` | 13 | 13 | 0 | 0 | 6 |
| `tests/test_reference_licence.py` | 154 | 125 | 3 | 26 | 8 |
| `tests/test_request_validation.py` | 23 | 23 | 0 | 0 | 3 |
| `tests/test_retention_bookkeeping.py` | 17 | 17 | 0 | 0 | 2 |
| `tests/test_retry_varies_the_question.py` | 6 | 6 | 0 | 0 | 0 |
| `tests/test_score_and_length.py` | 11 | 11 | 0 | 0 | 4 |
| `tests/test_service_errors.py` | 6 | 6 | 0 | 0 | 0 |
| `tests/test_shot_planning.py` | 19 | 8 | 0 | 11 | 0 |
| `tests/test_storyboard_role.py` | 5 | 3 | 0 | 2 | 0 |
| `tests/test_structure_role.py` | 5 | 3 | 0 | 2 | 0 |
| `tests/test_style_role.py` | 5 | 3 | 0 | 2 | 0 |
| `tests/test_supported_python_versions.py` | 2 | 0 | 0 | 2 | 0 |
| `tests/test_swap_roles.py` | 59 | 48 | 0 | 11 | 4 |
| `tests/test_upload_endpoint.py` | 38 | 38 | 0 | 0 | 0 |
| `tests/test_upload_store.py` | 31 | 31 | 0 | 0 | 0 |
| `tests/test_video_reference.py` | 10 | 8 | 0 | 2 | 0 |
