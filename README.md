# ContextIR

独立、标准库实现的 Go module：将多模态创作意图表示为可编辑、可持久化的类型化 IR，再编译为 MiniMax H3 的公开提示词格式与素材绑定。

```text
用户请求 + 素材事实 → Planner（注入模型）→ Document
                                         ↓
                              Validate → H3 Compile
                                         ↓
                             prompt + manifest + hashes
```

`Document` 是唯一事实来源。可以手工创建、从 JSON 加载，或者让 Planner 生成；最终 prompt 不会由另一次模型写作绕过 IR。库不依赖 CLW Studio、Python、数据库、队列或特定模型 SDK。

这是基于公开格式的独立实现，不是 MiniMax 托管 H3-Context-IR 的内部实现。单元测试通过不代表生成质量与官方等效。

## 安装

Go 1.25 或以上。本仓库为私有仓库，调用方需要相应 GitHub 读取权限；按自己的环境配置 `GOPRIVATE=github.com/ly95/*`，不要将访问凭据写进代码或 module URL。

```sh
go get github.com/ly95/contextir@v0.1.0
```

生产调用方应固定经过验收的提交或版本。

## 手工创建与编译

```go
doc := contextir.Document{
    Version: contextir.Version,
    Mode:    contextir.ModeText,
    Target: contextir.Target{
        Duration: 8000,
        AspectRatio: "16:9",
        FPS: contextir.Rational{Numerator: 24, Denominator: 1},
    },
    Shots: []contextir.Shot{{
        ID: "opening", Start: 0, End: 8000,
        Description: contextir.Text("A baker opens the wooden shutters."),
    }},
    Soundscape: "Wooden shutters scrape open.",
    Music: "N/A",
}
artifact, err := h3.Compile(doc, h3.OfficialProfile())
if err != nil {
    // errors.As(err, &typedError) 获取稳定 Code、Stage 和字段诊断。
    return err
}
// artifact 是完整编译产物；交由调用方提交给相应 H3 推理适配器。
_ = artifact
```

导入 `github.com/ly95/contextir` 和 `github.com/ly95/contextir/h3`。完整可运行示例：

```sh
go run ./examples/manual
```

## 使用模型规划

`planner` 维护提示词、JSON Schema、原始请求保护、事实出处校验和可配置的有限重试。实际模型选择、API 凭据、媒体访问和计量由调用方适配器负责。

```go
p, err := planner.New(myModel, myPerceiver, planner.Options{MaxRetries: 1})
if err != nil { return err }
result, err := p.Plan(ctx, request)
// 即使 err != nil，仍检查 result.Calls 以保留已发生调用的用量。
if err != nil { return err }
artifact, err := h3.Compile(result.Document, h3.OfficialProfile())
```

实现两个窄接口：

```go
type Model interface {
    Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)
}
type Perceiver interface {
    Perceive(context.Context, contextir.Asset) (contextir.Evidence, error)
}
```

`ModelRequest` 提供阶段、system prompt、结构化 JSON 输入与完整 JSON Schema。模型适配器必须明确支持所需能力；不得默默切换模型、移除 schema 或忽略模态。`Perceiver` 需要明确描述观察范围和局限；视频抽三帧不能被标记为完整动作理解，转写也不能被当作完整音频理解。

已有素材理解结果可调用 `PlanWithEvidence(ctx, request, evidence)`。缺少的素材才交给 Perceiver；既没有有效证据、也没有感知能力时会明确失败。感知费用由 Perceiver 适配器单独计量，`result.Calls` 只覆盖规划模型调用。

请求的模式、目标参数、素材及顺序、固定对白和约束受到保护。模型不能更改它们。`AllowCreativeAdditions` 显式控制是否允许创意补充。观测事实、用户要求、创意补充分别记录在 Claims 中；机器校验出处关系，不能证明自由文本的语义蕴含关系。

## IR 表达能力

- 五种模式：`t2va`、`i2va`、`l2va`、`fl2va`、`ref2va`。
- 稳定素材 ID、视觉/音轨显式选择、源区间、元数据与内容哈希。
- 主体、外观/动作/镜头等来源职责、保留/迁移/复用关系。
- 图片锚点、源视频、同步音轨与独立音频分别绑定。
- 连续镜头时间线、结构化引用 spans、视觉/动作/摄影机/声音事件。
- 全局说话人、原文对白、跨镜头连续对白、截断标记、旁白与可见文字。
- 环境声、配乐、静音/无配乐/无对白等显式限制。
- 持久化事实 Evidence、来源 Claims 和版本化 JSON Schema。

`RichText` 用显式 Span 引用主体/参考素材。不要在普通文字中手写 `<Picture N>` 之类的内部编号；由 H3 编译器统一分配。

## 时间与运行时配置

IR 时间使用整数毫秒。H3 适配器输出 24 FPS 的精确帧数与有理数播放时长。

- `OfficialProfile()` 针对公开提示词格式，不代表托管 API 或某台推理机器的完整能力合同。
- `ComfyUIProfile()` 额外检查 `17k+5` 帧网格与相应输入限制；该网格不是通用 IR 规则。
- 编译器从不自动调整时长。无法精确换算时，调用方必须显式提供 `Target.FrameCount`，并让 `Duration` 等于该帧数播放时长的最近毫秒表示。
- 源素材裁剪、下载、转码、精确帧条件和实际 socket 连接由调用方执行。manifest 说明应该绑定什么，不会私自操作媒体。

## JSON 与命令行

```sh
go run ./cmd/contextir schema > schema/contextir-v1.schema.json
go run ./cmd/contextir validate < document.json
go run ./cmd/contextir compile -profile official < document.json
go run ./cmd/contextir compile -profile comfyui < document.json
```

`Decode` 拒绝未知字段、大小写别名、重复 JSON keys、尾随数据、无效 UTF-8 和不成对的 Unicode surrogate。默认文档上限是公开常量 `MaxDocumentBytes`（8 MiB）；更大文档需显式调用 `DecodeWithOptions`，超限会报错，不会截短。

`Decode` 检查 wire shape，`Validate` 检查跨引用与时间等合同；`Encode` 在验证后输出 JSON。独立 Go API 版本和文档的 `contextir/v1` 版本分别管理。`schema/contextir-v1.schema.json` 从 Go 类型生成并由测试检查同步。

## 测试与质量边界

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run=^$ -fuzz=FuzzDecode -fuzztime=10s .
```

测试不访问真实模型，不消耗 API 费用。它们验证数据合同、编译、原文保真、素材绑定和错误传播。真实模型的理解质量与最终视频质量需要单独的多案例、固定参数和种子对照；本仓库尚未建立与官方系统的生成质量等价证据。

## 资料与来源

以 MiniMax 官方 [基础指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md) 和 [全参考指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md) 为格式依据，并参考 [OpenH3-IR 固定提交 fd031e1](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5) 的类型、编号、测试与工程经验。详见 [UPSTREAM.md](docs/UPSTREAM.md) 与 [架构说明](docs/ARCHITECTURE.md)。
