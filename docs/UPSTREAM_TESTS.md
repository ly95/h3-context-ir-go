# 上游行为与 Go 测试

参考版本固定为 OpenH3-IR 提交 [`fd031e136ae3d89147324c0d1b2aa65e838c21f5`](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5)。其原始清单包含 42 个文件、991 个参数化测试条目；条目数不等于独立功能数。

[matrix.json](../conformance/matrix.json) 保留原 nodeid、断言、来源位置和 Go 适配映射；[adaptations](../conformance/adaptations) 解释输入、接口和行为差异。这些是静态需求与追溯信息，不能用它们的存在推断当前测试已经执行或通过。

## 当前复验入口

```sh
make check
```

该入口执行格式检查、vet、全部原生测试、race 与解码 fuzz。全部检查使用 Go，不需要 Python、模型凭据或历史验收报告。媒体集成测试需要 ffmpeg/ffprobe，race 需要 C 编译器；`make check` 在媒体工具缺失时先失败。直接运行 `go test ./...` 会明确跳过缺少工具的媒体用例，不能代替完整检查。普通测试使用受控依赖、固定上游输入和必要的模型响应录制，不发起付费调用。

按领域定位测试可使用：

```sh
go test ./compiler ./planner ./h3 ./h3check
go test ./plan ./budget ./tokens ./evaluate
go test ./openai ./chatcompat ./perception ./media ./service ./uploads ./comfy ./lora
go test ./cmd/... ./conformance/bridge
```

## 按原意移植

先读取原断言、理由和失败反例，再通过 Go 公共入口表达同一业务规则。允许调整接口、合法模式、显式依赖和后端 profile，前提是保留原来的成功场景与反例。不能用一个拒绝用例代替原测试保护的成功能力，也不能把执行后端不支持的能力写成已经实现。

| 保护的规则 | Go 验证方式 |
| --- | --- |
| 用户镜头数量优先于导演偏好 | 公共 Compile 保留用户与导演原请求，产出用户指定镜头数量；违约结构和最终产物均被拒绝。 |
| 正文中的每个媒体引用有真实绑定 | 显式注入观察后执行 Draft/Compile，检查 Artifact 标签与 manifest；虚构标签、越权绑定和错误来源有负例。 |
| 普通主体不继承临时姿态，首尾帧保留完整像素 | 对相同观察分别采用普通参考和实际帧职责；在支持能力的 profile 下检查成功，在受限后端检查明确拒绝。 |
| 多来源属性有独立来源，环境和风格不变成演员 | 检查实际主体、来源图、投影、属性许可、正文使用和 Artifact；不把 note 或目标文字伪装成观察。 |
| 固定对白与可见文字逐字保真 | 检查最终 Document 与 H3 产物、镜头范围、连续分片和额外内容；跨镜分片必须逐字节拼接回原文。 |
| 复制音轨与引用属性有不同权限 | 检查 signal copy、voice/music/beat/SFX 的实际接收对象、说话人关系和新增内容边界。 |
| 合同来自同一份定义并明确版本 | HTTP、Go 接口、JSON/JS 导出和实际 CLI 输出一致，字段变化受版本与摘要测试约束。 |
| 修复有界且用量不丢失 | 真正经过 Planner/Compiler 的模型合同边界；成功修复、预算耗尽、取消、无效计量与传输错误分别验证。 |
| 声明的 Go 版本可支持源码 | 解析实际 Go 源码并检查语言及标准库 API 版本；CI 使用声明的最低工具链运行全套检查。 |

`compiler/upstream_*_contract_test.go`、`compiler/remaining_*_test.go` 与各包的 `upstream_test.go` 保留这些行为验证。根目录的 Go 兼容性测试验证 Go 源码规则，不机械翻译 Python 的 f-string 语法约束。

## 确定性语料与录制回放

[conformance/fixtures](../conformance/fixtures) 保存原输入、原函数输出、命名 controls、原断言来源及单独标注的补充 probe。Go corpus tests 直接验证对应的原生 API。某个组件的输出一致不能自动升级为完整 Compile 或成片质量一致。

高层编译录制放在 [compiler/testdata](../compiler/testdata)。严格回放检查 Stage、System、Input 和 Schema 后才消费响应，保留失败候选与实际调用顺序。回放不访问模型服务，不是当前模型的一次新实测。桥接工具的边界测试另外验证错误请求不得消费录制，以及 provider 拒绝的类型与用量不能丢失。

真实模型录制使用单独的 [live-eval 命令](../cmd/h3-context-ir-go-live-eval/README.md)。模型安全输入与期望断言分开保存；成功响应、结构校验、最终产物语义和实际视频质量分别评估。

## 历史对照与当前检查的区别

开发期曾将未修改的原 Python 断言与 Go 输出对照，并另用 Go API 验证接口和输入适配。该审计对 991 个原要求形成了历史覆盖结论；原样 Python 桥接仍保留十项版本、CLI、模式设置、观察依赖、profile 或媒介分类差异，不能改写为原断言全部通过。

当前发布树不包含 Python 桥接、历史运行报告或重放完整历史审计所需的脚本。运行上述 Go 命令验证的是当前原生实现及其测试，不能称作重新执行了全部 991 个未修改的原 Python 断言。需求清单中保留的历史状态同样不能替代一次当前执行。

H3 Context-IR Go 不以测试数量证明与 MiniMax 托管 Context-IR 内部实现等价。真实 H3 推理、人物一致性、音画同步、原文字形和来源像素保真需要额外的生成视频评测。
