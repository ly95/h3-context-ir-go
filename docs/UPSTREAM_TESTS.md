# 上游完整行为验收

固定上游原清单的 **989 项 Go 适用要求全部通过**；另外 2 项 Python 语法检查不适用，未计入通过数。来源是 42 文件、991 个原 nodeid，不以自编示例代替原要求。

本版已发布并从 GitHub 私有远端独立下载：`v0.1.1-0.20261020120832-874c5ee273bc`。下载包内 370 个主测试、7806 个子测试全部通过且无跳过，见 [外部全套证据](../conformance/reports/private-native-remaining-final-v4.json) 与 [最终汇总](../conformance/reports/completion-remaining-v3.json)。

[最终逐项报告](../conformance/reports/native-functionality-remaining-all-v3.json) 对每个要求保存实际验证入口、原断言意图、输入调整和证据：**672 项由未修改的原断言检查实际 Go 主输出，317 项用独立 Go 接口测试适配**。没有遗漏、重复、跳过或把辅助函数触达当作主功能通过。独立 Go 测试与原断言使用同一生产实现；受控模型候选和严格录音回放不需要付费服务。

原样 Python 断言重跑依然是 **981 passed / 10 failed，0 skipped**，见 [完整原报告](../conformance/reports/go-draft-pytest-remaining-final-v1.json)。这不是“991 原断言全部通过”。十项 Go 接口差异另有实际正反例验证，原失败保持红色。此前 986/5 和其他所有历史轮次继续保留。

## 证据怎样计数

| 验证入口 | 原 nodeid 数 | 通过条件 |
| --- | ---: | --- |
| `original_go_primary` | 672 | 原 call phase 通过；该 nodeid 要求的主 Go operations 全部实际执行；需要的 validator 调用确实发生；不可变 bridge、当前 runtime、适配器、fixture 与真实录音请求来源核验通过 |
| `go_test` | 317 | 原要求映射到实际执行并通过的 Go 测试叶；按 Go 接口明确记录合法模式、观察依赖、类型化输入或版本差异；配套失败/越权/保真对照 |
| `not_applicable_go` | 2 | Python 模块语法、f-string 语法；Go 编译和 vet 单独检查，不能计作这两条原断言通过 |

317 项包含旧 126 个 Python-only 节点中的 124 个适用要求，以及 193 个此前已经触达 Go、但主断言仍需独立验证或接口适配的节点。672 项没有虚构独立 Go test leaf：入口仍是原 source nodeid 与实际 native operation。逐项 [适配清单](../conformance/adaptations/remaining_existing_native.json) 与其他 `remaining_*.json` 列出原断言、来源 SHA、边界和精确测试/fixture IDs。

[最终本地质量检查](../conformance/reports/local-final-quality-v3.json) 的全仓 race、vet、gofmt、diff 与 conformance 门卫全部通过。最终聚合也执行全部 Go 测试并核验运行期间源文件不变；原始 Go JSON events、源码归档和 hash 在报告中。首轮 [v1](../conformance/reports/native-functionality-remaining-all-v1.json) 保持不变，不把两轮事件数相加。

## 原样重跑保留的十项差异

以下原测试均仍失败；表中 Go 测试是另一个明确记录的验证入口，不会改写原结果。

| 原 nodeid（省略共同 `tests/`） | 真实差异与 Go 验证 |
| --- | --- |
| `test_contract.py::test_the_service_publishes_one_statement_of_the_lists_it_shares_with_the_contract` | 原 contract v2 对照 native **v3**。`TestRemainingSurfaceContractVersionAndCapabilities` 检查真实版本、完整字段/错误码/能力，包括新增字段；旧 v2 表仅作来源子集 oracle。 |
| `test_contract.py::test_the_version_moves_when_any_part_of_the_contract_moves` | v3 字段与拒绝码使实际 digest 改变，不能伪装旧 v2。上述 Go 测试同时校验 source oracle 与当前 native 合同。 |
| `test_contract.py::test_the_command_prints_what_the_module_builds` | 原 subprocess 调用 Python CLI；`TestRemainingSurfaceContractCLIPrintsPublishedBytes` 真正构建并运行 Go CLI，检查 JSON/JS 完整 stdout 与公开 Go module 输出一致。 |
| `test_contract.py::test_the_generated_module_carries_the_moves_and_the_cap` | 原 JS 期望 Python v2。`TestRemainingSurfaceGeneratedJS` 检查实际 native v3 的 moves、cap、version 与 digest。 |
| `test_contract.py::test_the_generated_module_says_it_is_generated_and_how_to_regenerate_it` | 原注释要求 `h3ir contract --js`；native 注释公布实际 `contextir contract -js` 和 Go 再生命令，同一 Go 测试验证。 |
| `test_director.py::test_direction_cannot_move_a_shot_count_the_caller_pinned` | 空素材 `ref2va` 在入口拒绝。`TestGoConformancePinnedShots` 用合法 `t2va` 检查用户 2 镜优先于导演 3 镜，错误模型结构/成品不能覆盖用户控制。 |
| `test_hardening.py::test_every_label_in_the_text_has_a_manifest_entry_behind_it` | 原调用缺少观察依赖；note 不作观察。`TestGoConformanceManifest` 保持原 ref1.png/需求/hash，显式注入归档真实观察，检查实际 Draft → Artifact 非空 manifest、正文引用及 phantom 负例。 |
| `test_reference_licence.py::test_a_frame_anchor_DOES_licence_its_pose` | 原 ComfyUI profile 不支持该 exact reference anchor。`TestGoConformanceFramePose` 在 OfficialProfile 下保留 first-frame → `keyframe@0`、完整保留及 pose；subject 不获得 pose；同一文档在 ComfyUI 明确拒绝。 |
| `test_reference_licence.py::test_a_frame_anchor_is_exempt_from_a_transformation_marker` | 原 `fl2va` 只有一张图，入口拒绝。`TestGoConformanceRetention` 使用合法 subject/首帧/尾帧模式，并拒绝恶意 frame retention；没有改变模式或丢锚点求通过。 |
| `test_reference_licence.py::test_the_rule_is_NOT_armed_when_the_requested_medium_is_unclassifiable` | 原词表要求 `1990s cel animation` 为 None；native typed 决策可识别为 2D。`TestGoConformanceMediumUnknownClassificationDoesNotProveRendition` 保留完整目标风格，区分未知分类、具体变换和跨类退回。 |

前五项映射见 [surface](../conformance/adaptations/remaining_surface.json)、[真实 CLI](../conformance/adaptations/remaining_cli.json)、[native JS](../conformance/adaptations/remaining_js.json)；后五项见 [现有 Go 契约映射](../conformance/adaptations/remaining_existing_native.json) 和 [原五项调整说明](../conformance/go_adapted_cases.json)。新增负例曾发现实际 frame-source 角色错误与非法 retention 漏检，[旧源码 RED](../conformance/reports/go-adapted-before-fixes-red-v1.json) 保留修复前失败，未通过放宽断言求绿。

## 修复、对白与保真

[57 项修复/跨镜映射](../conformance/adaptations/remaining_repair.json) 覆盖原 fix-loop、跨镜对白、帧表示、指令行和前缀修复。测试真正走 Go Planner 的有限 schema/协议修复与实际 H3 Artifact，同时验证顽固错误耗尽预算、逐次用量和失败；生成稿格式 helper 必须显式调用，并返回修改记录，不自动改写用户字面内容。

用户提出的中文回归另列为补充测试 `TestRemainingRepairUserChineseTwoFourSecondShots`，不占原 989 项：原文“我们明天见，好吗？”，两镜分别 `[0,4000)` / `[4000,8000)` ms；合法片段“我们明天见，”与“好吗？”逐字节拼接一致，两个连续标记配对，同一说话人、没有额外对白或配乐。测试先拒绝重复整句候选，再验证真实 Planner/H3 产物。它证明这些结构证据成立，不是新 DeepSeek 或视频质量测量，见 [运行记录](../conformance/reports/go-remaining-repair-v2.json)。

## 当前模型录音与真实高层编译

当前合同的许可录音主批次为 437 entries / 228 unique，其中 1 unique（14 个复用 entries）失败；[主报告](../conformance/reports/remaining-full-licence-v1/report.json) 保持原失败。同合同 [显式重试](../conformance/reports/remaining-full-licence-retry-v1/report.json) 14 entries / 1 unique 成功，另有 [10 unique 补录](../conformance/reports/remaining-full-licence-supplement-v1/report.json)。新索引保留 461 条记录、238 个完整输入，按 namespace 保留 reuse chains，不混入旧 System 合同的补充录音。

本次原断言重放的 423 次 licence/style 调用，对全部 **1,422 次实际 recorded Stage/System/Input/Schema** 在 Go 中核验后才消费。真实非法 JSON、其用量与后续修复请求也按原错误重放；没有删掉坏响应或把未验证历史结果当当前运行。fresh bridge SHA 为 `8e7cd974c82d…`，[完整来源证明](../conformance/reports/go-draft-pytest-remaining-final-v1-provenance.json) 记录 source archive、当前 runtime、适配器、fixture 和录音文件 hash。

[最新三例高层 Compile 独立验读](../conformance/reports/remaining-highlevel-compile-v2-verification.json) 全部 compiled 并通过各自主要要求：两例转写“`We close at six, not half past.`”各恰有一条同语言固定对白和一次最终 prompt；身份组合得到一个 sports car，两个真实来源，明确区分观察到的 carbon fibre body 与用户要求的 wheels 属性。自包含 20 次阶段请求严格回放及 drift 负例纳入 Go 测试；转写例仍有 R5/A2/token-band 等 WARN，模型选择例还有 P3，不称全面质量通过。

旧 [六例 suite](../conformance/reports/live-suite-round8/report.json) 和 [16 角色 Compile](../conformance/reports/live-roles-round4/report.json) 分别为 6 compiled 与 15 compiled/1 failed；后续 [environment](../conformance/reports/live-roles-environment-round1/report.json) 与 [animate](../conformance/reports/live-suite-style-round1/report.json) 单例另存。20 个旧版本结果加 2 个新版本补录不能称 22 例同一 binary。suite 使用真实 PNG 像素；角色 cards 是原测试合成数据，不能当真实视频/音频观察。角色全 Compile 省略原 deterministic_draft 独立 mode 参数，[调用合同](../conformance/reports/roles-compile-contract.json) 明示这种比较局限。

旧轮次语义错误、非法 JSON、失败和 WARN 全部保留。IR/格式通过、compiled、自动 review 或人工审查都不能证明任意描述的语义完备，更没有实际 H3 视频生成或成片一致性证据。

## 复验与固定来源

```sh
# 普通测试不读取模型凭据。
go test ./...
go test -race ./...
go vet ./...
python3 conformance/run_native_functionality.py --require-all \
  --original-report conformance/reports/go-draft-pytest-remaining-final-v1.json \
  --label a-new-native-round
```

聚合 runner 拒绝复用标签、遗漏、重复、跳过、主 API 未执行、不匹配的 bridge/runtime/fixture/录音请求及运行期间源码改变。原报告的 10 项失败不会被变为原通过。完整干净上游环境、冻结 bridge 和显式 licence index 的重跑步骤见 [conformance/README](../conformance/README.md)。

上游固定为 [`fd031e136ae3d89147324c0d1b2aa65e838c21f5`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)，Python 0.4.1。原 23 controls、17 planted-defect RED 与 991 Python 基线已原样执行；它们各自证明来源/反例，不计作 Go 验收。386 validator、1,952 plan/mode、1,617 licence/style、1,927 render/text 实际调用保留完整参数、assert/nodeid 和 oracle。captured corpus 的输入数不等于整 Compile 通过数。

`source_manifest.json` 保存每个上游 tracked 文件和复制资产 hash；11 个 golden/PNG 字节不变，保留 Apache-2.0 与 NOTICE。运行时只有 Go 标准库；模型期望不进入模型输入。下载模块不含大体积 reports，但测试使用的小型录音/testdata 自包含，不依赖 reports 目录。

## 保留的历史证据

| 运行 | 当时结果 | 当时边界，不作当前全量结论 |
| --- | ---: | --- |
| [原 Python 基线](../conformance/reports/upstream-pytest.json) | 991 passed | 0 Go；42 文件、0 skipped |
| [validator round3](../conformance/reports/go-validator-pytest-round3.json) | 991 passed | 295 Go nodeid，696 Python |
| [core round19](../conformance/reports/go-core-pytest-round19.json) | 991 passed | 770 Go nodeid，原 Draft/renderer 仍 Python |
| [full plan round3](../conformance/reports/go-plan-pytest-round3.json) | 991 passed | 312 次 native plan.Build，mode 仍由 Python 决定 |
| [972a 组合](../conformance/reports/go-combined-pytest-972a-supplement-complete.json) | 990 passed / 1 failed | 857 触达 Go，尚未替换完整 Draft |
| [wholeDraft 旧五差异轮](../conformance/reports/go-draft-pytest-go-adapted-contracts-v1.json) | 986 passed / 5 failed | 865 触达，126 Python-only，不等于主行为全部 native |
| [当前原样重跑](../conformance/reports/go-draft-pytest-remaining-final-v1.json) | 981 passed / 10 failed | 原结果保持；当前全部要求另按 672 + 317 + 2 分类 |

`matrix.json` 采用 additive 更新：旧 pending、触达数和历史状态原样保留，当前结论位于 `current_native_primary_verification` 与每项 `native_primary_verification`。不能把旧覆盖并集、回放历史结果和当前原生主输出混算。

## 最终逐文件分账

“原断言→Go”与“Go 适配”均为已执行并通过的要求；最右列是另一次未修改 Python harness 的原始结果，不同验证入口不相加。

| 上游文件 | 原项数 | 原断言→Go | Go 适配 | Python NA | 原样 passed / failed |
| --- | ---: | ---: | ---: | ---: | ---: |
| `tests/test_anchor_downgrade_is_honest.py` | 5 | 0 | 5 | 0 | 5 / 0 |
| `tests/test_ask_states_the_wiring.py` | 10 | 0 | 10 | 0 | 10 / 0 |
| `tests/test_audio_card_is_not_cached.py` | 5 | 5 | 0 | 0 | 5 / 0 |
| `tests/test_audio_claims.py` | 19 | 15 | 4 | 0 | 19 / 0 |
| `tests/test_capacity.py` | 13 | 10 | 3 | 0 | 13 / 0 |
| `tests/test_closed_vocabulary.py` | 17 | 16 | 1 | 0 | 17 / 0 |
| `tests/test_contract.py` | 22 | 10 | 12 | 0 | 17 / 5 |
| `tests/test_creativity.py` | 57 | 51 | 6 | 0 | 57 / 0 |
| `tests/test_declared_roles.py` | 21 | 18 | 3 | 0 | 21 / 0 |
| `tests/test_deterministic.py` | 58 | 36 | 22 | 0 | 58 / 0 |
| `tests/test_dialogue_across_a_cut.py` | 25 | 0 | 25 | 0 | 25 / 0 |
| `tests/test_director.py` | 38 | 13 | 25 | 0 | 37 / 1 |
| `tests/test_endpoint_portability.py` | 43 | 42 | 1 | 0 | 43 / 0 |
| `tests/test_enrichment_path_imports.py` | 4 | 0 | 4 | 0 | 4 / 0 |
| `tests/test_every_role_renders.py` | 40 | 37 | 3 | 0 | 40 / 0 |
| `tests/test_ffmpeg_absent.py` | 3 | 3 | 0 | 0 | 3 / 0 |
| `tests/test_file_facts.py` | 26 | 17 | 9 | 0 | 26 / 0 |
| `tests/test_fix_loop.py` | 24 | 0 | 24 | 0 | 24 / 0 |
| `tests/test_frame_anchors.py` | 19 | 15 | 4 | 0 | 19 / 0 |
| `tests/test_golden_asset_geometry.py` | 2 | 0 | 2 | 0 | 2 / 0 |
| `tests/test_hardening.py` | 49 | 44 | 5 | 0 | 48 / 1 |
| `tests/test_instruction_line.py` | 19 | 15 | 4 | 0 | 19 / 0 |
| `tests/test_megapixels.py` | 6 | 6 | 0 | 0 | 6 / 0 |
| `tests/test_music_referenced_not_copied.py` | 37 | 26 | 11 | 0 | 37 / 0 |
| `tests/test_paired_soundtrack.py` | 8 | 8 | 0 | 0 | 8 / 0 |
| `tests/test_pinned_shots.py` | 17 | 10 | 7 | 0 | 17 / 0 |
| `tests/test_reference_audio_words.py` | 13 | 7 | 6 | 0 | 13 / 0 |
| `tests/test_reference_licence.py` | 154 | 110 | 44 | 0 | 151 / 3 |
| `tests/test_request_validation.py` | 23 | 20 | 3 | 0 | 23 / 0 |
| `tests/test_retention_bookkeeping.py` | 17 | 15 | 2 | 0 | 17 / 0 |
| `tests/test_retry_varies_the_question.py` | 6 | 6 | 0 | 0 | 6 / 0 |
| `tests/test_score_and_length.py` | 11 | 7 | 4 | 0 | 11 / 0 |
| `tests/test_service_errors.py` | 6 | 6 | 0 | 0 | 6 / 0 |
| `tests/test_shot_planning.py` | 19 | 6 | 13 | 0 | 19 / 0 |
| `tests/test_storyboard_role.py` | 5 | 3 | 2 | 0 | 5 / 0 |
| `tests/test_structure_role.py` | 5 | 3 | 2 | 0 | 5 / 0 |
| `tests/test_style_role.py` | 5 | 3 | 2 | 0 | 5 / 0 |
| `tests/test_supported_python_versions.py` | 2 | 0 | 0 | 2 | 2 / 0 |
| `tests/test_swap_roles.py` | 59 | 15 | 44 | 0 | 59 / 0 |
| `tests/test_upload_endpoint.py` | 38 | 38 | 0 | 0 | 38 / 0 |
| `tests/test_upload_store.py` | 31 | 31 | 0 | 0 | 31 / 0 |
| `tests/test_video_reference.py` | 10 | 5 | 5 | 0 | 10 / 0 |
| **合计** | **991** | **672** | **317** | **2** | **981 / 10** |
