# 上游完整行为验收

发布目标为 `v0.2.0`；当前验收覆盖固定上游 `fd031e136ae3d89147324c0d1b2aa65e838c21f5` 的 **42 文件、991 个原 nodeid**。nodeid 追溯原业务规则，不要求复制 Python 内部常量、函数形状或分类器局限。

[最新逐项报告](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/native-functionality-release-v2.json) 实际通过 **991/991，672 original_go_primary + 319 go_test，N/A 0**，无遗漏、重复或 Go 测试跳过。[全仓质量](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/local-quality-release-v1.json) 与 [真实最低 Go 1.25.0](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-minimum-release-v1.json) 均通过，442 主测试、7957 子测试。当前运行时包含对白、语义审核、参考角色/身份边界和时间校验修复，不能称作本轮仅添加测试。

[当前原样 Python 断言 v8](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-draft-pytest-release-final-primary-v8.json) 仍是 **981 passed / 10 failed、0 skipped**，其 [proof](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-final-primary-v8-verification.json) 核验 672 项实际 Go 主输出。十项原红色记录各有显式 Go 业务验收，不能改写成原断言通过。此前 v4 的 978/13、670/672、v5/v6 proof 和所有旧失败保持历史状态。

最终 `v0.2.0` 标签已在全新缓存中从私有远端实际安装，完成 442+7957 全套测试和完整 40 操作/277 尝试重放。精确 OriginHash、10,786,780 字节 ZIP 和安装后测试证明记录在 [main 发布索引](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json)。旧伪版本的分发证明仅作历史保留。

## 证据怎样计数

| 验证入口 | 原 nodeid 数 | 通过条件 |
| --- | ---: | --- |
| `original_go_primary` | 672 | 原 call phase 通过；该 nodeid 要求的主 Go operations 全部实际执行；需要的 validator 调用确实发生；不可变 bridge、当前 runtime、适配器、fixture 与真实录音请求来源核验通过 |
| `go_test` | 319 | 原要求映射到实际执行并通过的 Go 测试叶；按 Go 接口明确记录合法模式、观察依赖、类型化输入或版本差异；配套失败/越权/保真对照 |
| N/A | 0 | 无免除项；两项源码/声明版本兼容要求已按 Go 实现，并核验真实最低工具链全包测试 |

319 项包含旧 126 个 Python-only 节点的全部 126 个要求，以及 193 个此前已经触达 Go、但主断言仍需独立验证或接口适配的节点。672 项没有虚构独立 Go test leaf：入口仍是原 source nodeid 与实际 native operation。逐项 [适配清单](../conformance/adaptations/remaining_existing_native.json) 与其他 `remaining_*.json` 列出原断言、来源 SHA、边界和精确测试/fixture IDs。

[最终本地质量检查](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/local-quality-release-v1.json) 的全仓 race、vet、gofmt、diff 与 conformance 门卫全部通过。最终聚合也执行全部 Go 测试并核验运行期间源文件不变；原始 Go JSON events、源码归档和 hash 在报告中。首轮 [v1](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/native-functionality-remaining-all-v1.json) 保持不变，不把两轮事件数相加。

## Go 源码与最低版本兼容性

原两项 Python 语法测试的原意已按 Go 实现，不再列 N/A：`TestGoCompatibilityEverySourceParses` 解析实际 Go 源码，`TestGoCompatibilityDeclaredLanguageVersion` 检查 `go.mod` 声明版本对应的语言与标准库 API。负例包括当前编译器能读、较低语言版本拒绝的语法，以及较新标准库 API；原 Python f-string 规则没有伪称逐句翻译为 Go。

更强的 [真实最低 Go 1.25.0 全包测试与 vet](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-minimum-release-v1.json) 已通过，关闭自动升级与工作区，使用新缓存且未读取模型凭据。聚合门卫核对真实工具链、所有 Go/test/embedded 输入 hash、完整 JSON 事件、两项实际测试和运行期间证据不变；用 Go 1.27 或 Go 1.25.1 代替最低版本、筛选测试、跳过、篡改来源或日志均被拒绝。映射见 [remaining_go_compatibility.json](../conformance/adaptations/remaining_go_compatibility.json)。旧 989 项通过 / 2 项 N/A 的 [历史报告](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/native-functionality-remaining-all-v3.json) 保持原样。

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

前五项映射见 [surface](../conformance/adaptations/remaining_surface.json)、[真实 CLI](../conformance/adaptations/remaining_cli.json)、[native JS](../conformance/adaptations/remaining_js.json)；后五项见 [现有 Go 契约映射](../conformance/adaptations/remaining_existing_native.json) 和 [原五项调整说明](../conformance/go_adapted_cases.json)。新增负例曾发现实际 frame-source 角色错误与非法 retention 漏检，[旧源码 RED](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-adapted-before-fixes-red-v1.json) 保留修复前失败，未通过放宽断言求绿。

## Go 测试怎样保留上游意图

先读取原用例的断言、注释与失败反例，再用 Go 的公共入口、类型和显式依赖表达同一规则。调整设置必须保留成功场景和所保护的业务能力；调用方显式选择不能被运行时偷偷改变。测试中的受控模型输出只用于验证合同与错误处理，不作为真实模型质量证明。

| 上游保护的规则 | Go 验收应证明的行为 |
| --- | --- |
| 契约来自同一份定义，变化可被客户端识别 | 实际 HTTP、导入接口、JS 和真实 Go CLI 一致；完整 native 契约独立钉住摘要；增加字段、改变错误状态/路由、增删章节而不升级版本必须失败。Python v2 不是 Go 的固定版本要求。 |
| 用户镜头数高于导演偏好 | 同一模型请求保留用户 2 镜和导演 3 镜原文，并明确优先级；合法候选经公共 Compile 得到 2 镜成品，违约结构或成品被拒绝。无素材场景使用 t2va 不削弱此规则。 |
| 每个正文媒体引用都有实际绑定 | 显式提供观察后，真实 Draft/Compile 产物的正文标签与非空 manifest 双向对应；虚构引用和错绑失败。观察可注入，不能把用户 note 伪装成观察。 |
| 首帧姿势属于开场，普通参考姿势不属于身份 | 相同观察分别作为普通 subject 和首帧使用；普通 subject 保留姿态观察但不把它写进最终身份，首帧姿态进入最终产物。H3 格式能力在可表达它的 profile 下验证，后端能力限制另外验证。 |
| 风格转换不改变素材保留关系 | 普通主体、首帧、尾帧和合法双图 fl2va 均保持对应保留标记与锚点；源风格/目标风格对照一致，恶意 attribute_transfer 被拒绝。 |
| 未知媒介不能构成类别冲突证据 | 显式未知 source/target 不启用跨媒介误报，同时不能冒充目标风格已经实现；已知同类、已知跨类和退回源风格另有正反例。不要求重现 Python 对 cel animation 无法分类的局限。 |

原桥接的十项红色记录与上述 Go 行为验收回答不同问题，必须分别报告。原清单的 991 个参数化条目也不等于 991 种独立业务能力。

## 修复、对白与保真

[57 项修复/跨镜映射](../conformance/adaptations/remaining_repair.json) 覆盖原 fix-loop、跨镜对白、帧表示、指令行和前缀修复。测试真正走 Go Planner 的有限 schema/协议修复与实际 H3 Artifact，同时验证顽固错误耗尽预算、逐次用量和失败；生成稿格式 helper 必须显式调用，并返回修改记录，不自动改写用户字面内容。

用户提出的中文回归另列为补充测试 `TestRemainingRepairUserChineseTwoFourSecondShots`，不占原 991 项：原文“我们明天见，好吗？”，两镜分别 `[0,4000)` / `[4000,8000)` ms；合法片段“我们明天见，”与“好吗？”逐字节拼接一致，两个连续标记配对，同一说话人、没有额外对白或配乐。测试先拒绝重复整句候选，再验证真实 Planner/H3 产物。它证明这些结构证据成立，不是新 DeepSeek 或视频质量测量，见 [运行记录](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-remaining-repair-v2.json)。

## 当前模型录音与真实高层编译

[统一 release-live-round6](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6/report.json) 完整执行 **35 Compile + 5 Refine = 40 操作**：36 compiled、4 个预期拒绝；不是把旧不同 binary 的成功拼成一批。运行时 `a9f84ef09bc0c662bd8b0c52afac380a1ce10dc4` / `composition-review/v10`，binary SHA `2b34869092e0110606f3a071c18af4f6183e565e4237b8612d28c455d91b7bfa`。

配置为 `deepseek-flash` / `low` / 输出上限 16384 / 并行 3。原 34 Compile 显式 OfficialProfile、3/3 修复预算；新增 wardrobe 原例显式 ComfyUIProfile、1/1。共 277 实际调用、921,356 输入 token、538,309 输出 token。14 次后续尝试分为 host structured 12、provider protocol 1、semantic review 1；其中 Refine 的 3 次后续尝试最后仍为预期依赖拒绝，不是修复成功，Result.Calls 重复 ledger 不另算调用。详见 [完整清单](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-recording-inventory.json)。

[独立产物检查 40/40](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-verification.json) 与 [核心 21](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-core-review.json)、[角色 16](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-role-review.json)、[非法边界 3](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-semantic-boundary-review.json) 逐项复核全部通过。后三份 reviewer_kind 为 coding_agent_independent_artifact_review，不称人类签字或通用语义证明。

产品修复覆盖：中文固定对白的逐字节跨镜分片/连续标记及不可见旁白；真实 per-shot 对白/可见字叙事合同；属性音频的实际 recipient（voice 不能引导 engine SFX）；replacement 动作继承；角色与 PRE-projection 源图边界；非 actual-frame reference sheet 在 identity 后、licence owner 前的独立资格阶段与最终 canonical 审查；T12 整数毫秒比较。1200ms 不再被浮点误判，1199ms 仍拒绝；实际首尾帧的版页、标签和背景不被身份清理误删。

可见字按原文、typed display 数量/镜头及实际 prose 展示复核；同一标牌在同镜被描述两次不自动等于两次展示。[旧 oracle 更正](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round5-visible-text-oracle-correction.json) 保留旧词频失败。同镜两个标牌、跨镜 readable carry 和新增文字仍须拒绝。[checker 工具更正](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-live-round6-checker-tool-correction.json) 另存本轮首次 NameError 的源码/trace；新脚本整 inspect 路径正负对照通过后才得到实际 40/40。

prefix 的原属性名 `wheels from the second picture` 是 opaque selector。加前置来源后，typed graph 和明确 rendered source clause 仍把 wheels 绑定 asset-2 / Picture3；删除前置来源时按物理源重排，不能从属性名中的 ordinal 另猜绑定。summary 重复原属性名仍有自然语言位置歧义，报告明确保留；不声称全部散文无歧义、来源像素已由成片证实。

当前许可索引 [v2](../conformance/fixtures/release_final_licence_replay_resolutions_v2.json) 是原 461 entries 加 2 个补录，合计 463 entries / 238 完整输入。原主 437/228 unique 中的失败及 [retry](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/remaining-full-licence-retry-v1/report.json)、[supplement](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/remaining-full-licence-supplement-v1/report.json) 均不覆盖。[两个原 API 补录](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/release-helper-refresh-v1/report.json) 属于独立 v9 helper、binary `ff97523c…`、8 次调用、上限 12288；不是本轮 v10 main binary，不计入其 277 次。当前 replay 只有在完整 Stage/System/Input/Schema 匹配后消费各自来源录音。

[高层三例录音](../compiler/testdata/remaining_highlevel/manifest.json) 与 [wardrobe 完整 Compile](../compiler/testdata/remaining_wardrobe_fresh/manifest.json) 已用本轮完整响应更新，阶段数分别 6、6、10、12。这 34 条是 main 调用子集，严格回放没有新付费调用。Wardrobe 的原 licence-01108 明确适配为完整 Compile 后取实际 Licence 跑原 GuardOwnership，其余旧 wardrobe 录音仍保持原边界；原 Python 源 SHA/断言不变。

本批真实 PNG 用于 ref2 双主体、i2 动画和 paired 首尾帧；其他媒体 Card 是显式 supplied typed observations，不能称真实音视频感知。原词 TranscriptUses 的授权新表演不等于原音轨 signal copy，voice 属性也不自动授权台词。未执行实际 H3 视频推理，compiled、schema、自动 critic 与独立文本复核均不能证明任意语义或成片质量。

旧 [suite](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/live-suite-round8/report.json)、[16 角色批次](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/live-roles-round4/report.json)、[environment](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/live-roles-environment-round1/report.json)、[animate](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/live-suite-style-round1/report.json) 及其 [输入合同局限](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/roles-compile-contract.json) 继续作为历史证据。所有真实失败、非法 JSON、拒绝、WARN 和早期跨批补录均保持原状态。

## 复验与固定来源

```sh
# 普通测试不读取模型凭据。
go test ./...
go test -race ./...
go vet ./...
python3 conformance/run_native_functionality.py --require-all \
  --original-report conformance/reports/go-draft-pytest-release-final-primary-v8.json \
  --go-compatibility-report conformance/reports/go-minimum-release-v1.json \
  --label a-new-native-round
```

聚合 runner 拒绝复用标签、遗漏、重复、跳过、主 API 未执行、不匹配的 bridge/runtime/fixture/录音请求及运行期间源码改变。原报告的 10 项失败不会被变为原通过。完整干净上游环境、冻结 bridge 和显式 licence index 的重跑步骤见 [conformance/README](../conformance/README.md)。

上游固定为 [`fd031e136ae3d89147324c0d1b2aa65e838c21f5`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)，Python 0.4.1。原 23 controls、17 planted-defect RED 与 991 Python 基线已原样执行；它们各自证明来源/反例，不计作 Go 验收。386 validator、1,952 plan/mode、1,617 licence/style、1,927 render/text 实际调用保留完整参数、assert/nodeid 和 oracle。captured corpus 的输入数不等于整 Compile 通过数。

`source_manifest.json` 保存每个上游 tracked 文件和复制资产 hash；11 个 golden/PNG 字节不变，保留 Apache-2.0 与 NOTICE。运行时只有 Go 标准库；模型期望不进入模型输入。下载模块不含大体积 reports，但测试使用的小型录音/testdata 自包含，不依赖 reports 目录。

## 保留的历史证据

| 运行 | 当时结果 | 当时边界，不作当前全量结论 |
| --- | ---: | --- |
| [原 Python 基线](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/upstream-pytest.json) | 991 passed | 0 Go；42 文件、0 skipped |
| [validator round3](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-validator-pytest-round3.json) | 991 passed | 295 Go nodeid，696 Python |
| [core round19](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-core-pytest-round19.json) | 991 passed | 770 Go nodeid，原 Draft/renderer 仍 Python |
| [full plan round3](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-plan-pytest-round3.json) | 991 passed | 312 次 native plan.Build，mode 仍由 Python 决定 |
| [972a 组合](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-combined-pytest-972a-supplement-complete.json) | 990 passed / 1 failed | 857 触达 Go，尚未替换完整 Draft |
| [wholeDraft 旧五差异轮](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-draft-pytest-go-adapted-contracts-v1.json) | 986 passed / 5 failed | 865 触达，126 Python-only，不等于主行为全部 native |
| [当前原样重跑 v8](https://github.com/ly95/contextir/blob/bda66cc6b666fc7500b1c989a07b6c6f55adc71a/conformance/reports/go-draft-pytest-release-final-primary-v8.json) | 981 passed / 10 failed | 原结果保持；当前全部要求另按 672 + 319 分类，N/A 0 |

`matrix.json` 保留历史 pending、触达数和旧聚合状态；当前来源/映射在 `current_release_evidence`、`current_source_primary_verification` 与每项来源字段，最终实际通过结论以 native-functionality-release-v2 为准。快照中的 pending 表示捕获当时尚未执行的最终聚合，不能把旧覆盖并集或回放历史结果冒充新运行。

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
| `tests/test_supported_python_versions.py` | 2 | 0 | 2 | 0 | 2 / 0 |
| `tests/test_swap_roles.py` | 59 | 15 | 44 | 0 | 59 / 0 |
| `tests/test_upload_endpoint.py` | 38 | 38 | 0 | 0 | 38 / 0 |
| `tests/test_upload_store.py` | 31 | 31 | 0 | 0 | 31 / 0 |
| `tests/test_video_reference.py` | 10 | 5 | 5 | 0 | 10 / 0 |
| **合计** | **991** | **672** | **319** | **0** | **981 / 10** |
