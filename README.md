# ContextIR

独立、标准库实现的 Go module，当前处于实验验收阶段：将创作意图和素材观察表示为可编辑、可持久化的类型化 IR，再编译为 MiniMax H3 的公开提示词格式与素材绑定。

```text
用户请求 + 素材事实 → Planner（注入模型）→ Document
                                         ↓
                              Validate → H3 Compile
                                         ↓
                             prompt + manifest + hashes
```

`Document` 是唯一事实来源。可以手工创建、从 JSON 加载，或者让 Planner 生成；最终 prompt 不会由另一次模型写作绕过 IR。库不依赖 CLW Studio、Python、数据库、队列或特定模型 SDK。

这是基于公开格式的独立实现，尚未完整复现 MiniMax 托管 H3-Context-IR。已有 DeepSeek 文字与静态图片适配器；视频、音频感知和实际 H3 成片验收仍未完成。当前证据、已修复问题和缺口见 [READINESS.md](docs/READINESS.md)。

## 安装

Go 1.25 或以上。本仓库为私有仓库，调用方需要相应 GitHub 读取权限；按自己的环境配置 `GOPRIVATE=github.com/ly95/*`，不要将访问凭据写进代码或 module URL。

```sh
go get github.com/ly95/contextir@codex/readiness-audit
```

以上为开发验收分支。`v0.1.0` 是最初的基础版本，不包含后续审计修复或这里新增的 DeepSeek API；不会重写旧标签。生产调用方应固定经过自己验收的提交或版本。

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

`planner` 维护提示词、JSON Schema、结构化请求保护、事实出处校验和可配置的有限重试。模型与感知通过窄接口注入；仓库的 `deepseek` 包提供一个实际适配器。模型、凭据和媒体访问由调用方明确提供。

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

请求的模式、目标参数、素材及顺序、固定对白和约束受到结构校验保护。非空 `Request.Bindings` 锁定完整允许的来源与职责集合，包含首尾帧和源区间；非 nil Anchor 进一步锁定目标时间。未提供 Bindings 时，自由文本中的素材用途仍由模型解释。

默认拒绝新增对白。原文台词通过 `Request.Dialogue` 提供；确实需要模型生成台词时，显式设置 `AllowGeneratedDialogue`。`AllowCreativeAdditions` 拒绝未经授权、被模型标为 creative 的 Claims，但无法发现模型省略出处或错误归类的所有内容。Claims 还不是覆盖每个文字字段的完备依赖图；自由文本语义保真需要真实评测，详见 [planner 合同边界](planner/README.md)。

### DeepSeek 文字与图片

```go
client, err := deepseek.New(deepseek.Config{
    BaseURL: "https://api.deepseek.com",
    APIKey: apiKey, // 由宿主安全取得，不写入代码或文档。
    Model: "deepseek-flash",
    MaxOutputTokens: 8192,
    ReasoningEffort: "none",
})
if err != nil { return err }
perceiver, err := deepseek.NewImagePerceiver(client, resolveImage, deepseek.PerceptionOptions{
    OnCall: recordPerceptionUsage,
})
if err != nil { return err }
p, err := planner.New(client, perceiver, planner.Options{MaxRetries: 1})
```

导入 `github.com/ly95/contextir/deepseek`。`resolveImage` 的类型为 `func(context.Context, contextir.Asset) (deepseek.Image, error)`，由宿主显式提供完整图片的 data URI 或可访问图片地址。适配器不下载素材、不读取环境变量、不从文字 description 假造观察。纯文字请求不需要 Perceiver；图片观察的用量通过 `OnCall` 或 `PerceiveWithUsage` 单独取得。`UsageInvalid` 标识适配器已知无效的计量证据，不能据此收费。

默认要求完整 `Evidence.Coverage`。部分观察和旧版未声明覆盖必须通过 Planner Options 显式允许；不会升级成完整理解。内置 ImagePerceiver 对视频和音频返回 unsupported，不能用抽帧或转写假装覆盖全部媒体。

实际付费验收使用固定合成场景，输出每次调用与成功/失败记录：

```sh
# 事先由自己的安全配置准备 DEEPSEEK_API_KEY；不要写进命令参数。
go run ./cmd/contextir-eval -base-url https://api.deepseek.com \
  -model deepseek-flash -reasoning-effort low -max-retries 1 \
  -out /tmp/contextir-eval-new-run
```

输出目录必须不存在，`-max-retries` 默认 0。命令不会读取 dotenv；详细参数、语义人工检查标准与历史失败记录见 [evals](evals/README.md)。

## IR 表达能力

- 五种模式：`t2va`、`i2va`、`l2va`、`fl2va`、`ref2va`。
- 稳定素材 ID、视觉/音轨显式选择、源区间、元数据与内容哈希。
- 主体、外观/动作/镜头等来源职责、保留/迁移/复用关系。
- 图片锚点、源视频、同步音轨与独立音频分别绑定。
- 连续镜头时间线、结构化引用 spans、视觉/动作/摄影机/声音事件。
- 全局说话人、原文对白、跨镜头连续对白、截断标记、旁白、歌唱、复用音轨的歌词 cue 与可见文字。
- 环境声、配乐、静音/无配乐/无对白等显式限制。
- 持久化事实 Evidence、来源 Claims 和版本化 JSON Schema。

`RichText` 用显式 Span 引用主体/参考素材。不要在普通文字中手写 `<Picture N>` 之类的内部编号；由 H3 编译器统一分配。

## 时间与运行时配置

IR 时间使用整数毫秒。H3 适配器输出 24 FPS 的精确帧数与有理数播放时长。

- `OfficialProfile()` 针对公开提示词格式，不代表托管 API 或某台推理机器的完整能力合同。
- `ComfyUIProfile()` 额外检查 `17k+5` 帧网格、5–3600 帧范围、32 像素尺寸网格与输入限制；这些不是通用 IR 规则。socket 与数值约束有固定官方源码提取的合同 fixture。
- 编译器从不自动调整时长。无法精确换算时，调用方必须显式提供 `Target.FrameCount`，并让 `Duration` 等于该帧数播放时长的最近毫秒表示。
- 源素材裁剪、下载、转码、精确帧条件和实际 socket 连接由调用方执行。manifest 说明应该绑定什么，不会私自操作媒体。

## JSON 与命令行

```sh
go run ./cmd/contextir schema > schema/contextir-v1.schema.json
go run ./cmd/contextir schema -type evidence > schema/evidence-v1.schema.json
go run ./cmd/contextir validate < document.json
go run ./cmd/contextir compile -profile official < document.json
go run ./cmd/contextir compile -profile comfyui < document.json
```

`Decode` 拒绝未知字段、大小写别名、重复 JSON keys、尾随数据、无效 UTF-8 和不成对的 Unicode surrogate。默认文档上限是公开常量 `MaxDocumentBytes`（8 MiB）；更大文档需显式调用 `DecodeWithOptions`，超限会报错，不会截短。

`Decode` 检查 wire shape，`Validate` 检查跨引用与时间等合同；`Encode` 在验证后输出 JSON。Evidence 具有独立的 `DecodeEvidence`、`EvidenceSchema` 及对应严格合同。Go API 版本和文档的 `contextir/v1` 版本分别管理；发布 Schema 从 Go 类型生成并由测试检查同步。

## 测试与质量边界

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run=^$ -fuzz=FuzzDecode -fuzztime=10s .
```

以上默认测试不访问真实模型，不消耗 API 费用。它们验证数据合同、编译、原文保真、素材绑定和错误传播。真实 DeepSeek 合成测试单独运行、单独记录；最终视频质量尚无验收结果，也没有与官方系统的生成质量等价证据。GitHub Actions 的账户计费阻塞与本地检查分别记录在 READINESS 中。

## 资料与来源

以 MiniMax 官方 [基础指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md) 和 [全参考指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md) 为格式依据，并参考 [OpenH3-IR 固定提交 fd031e1](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5) 的类型、编号、测试与工程经验。详见 [UPSTREAM.md](docs/UPSTREAM.md) 与 [架构说明](docs/ARCHITECTURE.md)。
