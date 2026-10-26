# Changelog

## v0.2.0 — 2026-10-26

### 变更

- 规划修复默认使用 `planner.RepairValidatedDocument`。候选通过解码、锁定字段和核心校验，但未通过额外内容校验时，下一次修复同时接收该候选和结构化诊断。证据、内容权限与重试预算保持原有边界；失败的原始响应和过期候选不会被回放。
- 已固定镜头的修复 Schema 限定事件必须处于所属镜头的半开时间区间。没有素材的请求不能在修复中虚构媒体来源、参考关系或媒体保留元数据，普通用户或创意主体仍然可用。
- 参考模式的空摘要改为概述具有类型化目标出场关系的主体，不再复制镜头正文。被替换的原主体和未使用的定义不加入默认摘要；显式摘要与详细正文保留。
- 新增 [本地网页示例](examples/playground/README.md)：上传参考图、明确参考用途、比较优化前后、查看实际调用阶段、结构化诊断和用量，支持取消、复制与下载。
- 保存一轮真实 `deepseek-flash` 网页测试的截图、完整输入输出和调用记录；新增需要显式启用的真实模型验收入口，普通测试不产生付费 API 调用。

### 从 v0.1.0 升级

```sh
go get github.com/ly95/h3-context-ir-go@v0.2.0
```

- `planner.Options` 新增 `RepairProtocol` 字段。使用字段名初始化该结构体；旧的非键名结构体字面量需要修改。
- 默认修复输入、系统指令和修复 Schema 已变化。固定了旧模型合同的调用方可显式选择 `planner.RepairDiagnosticsOnly`；高层调用通过 `compiler.Options.Planner` 设置。这仅恢复旧修复协议，不恢复旧渲染器。
- `h3.CompilerVersion` 从 `contextir-h3/v1.2` 更新为 `contextir-h3/v1.3`。相同 Document 的文档哈希保持不变；空摘要变化会改变对应提示词、提示词哈希、渲染 token 预算及其派生诊断。所有重新编译产物的总哈希都会因编译器版本变化而改变。缓存和快照应区分编译器版本。
- Document 版本、Schema ID、H3 格式版本和 Go module path 保持不变。本次发布包含公开配置项与默认行为变化，因此使用新的次版本号。

### 验证与边界

发布检查使用 `make check`，包含格式、vet、全部原生与媒体测试、race，以及 Document 和 Evidence 解码的 fuzz。

[网页实测记录](examples/playground/docs/smoke-test.json)中，请求和返回模型均为 `deepseek-flash`，原始参考图实际参与感知，完整编译与语义审查通过。该轮有 10 次调用，耗时 31.1 秒，记录输入 34,729 tokens、输出 4,742 tokens。截图与优化前后全文见 [示例 README](examples/playground/README.md#真实运行截图)。记录只代表这一轮，不保证模型始终满足合同、视频质量或与 MiniMax 托管服务等价。本项目不执行实际视频生成。

## v0.1.0 — 2026-10-23

`github.com/ly95/h3-context-ir-go` 的首个发布版本。包含版本化 Document、类型化需求编译与修订、H3 渲染和校验、显式模型与感知适配、媒体准备及可运行的库示例。详细说明见 [v0.1.0 Release](https://github.com/ly95/h3-context-ir-go/releases/tag/v0.1.0)。
