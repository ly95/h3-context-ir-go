# H3 Context-IR Go

H3 Context-IR Go 是面向 MiniMax H3 的非官方 Context-IR Go 实现。它是独立的 Go 库，把自然语言需求与素材观察编译为可编辑、可持久化的 IR，再从同一份 `Document` 生成 H3 prompt、素材绑定、预算和内容哈希。

核心只使用 Go 标准库，不需要 Python；可选模型适配器使用官方 OpenAI Go SDK。模型、感知、存储和 HTTP 执行能力由调用方显式配置；核心验证与渲染可以完全离线运行。

本项目参考 [OpenH3-IR](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5) 与 MiniMax 公开的 H3 格式指南，是独立、非官方实现。它不包含 MiniMax 的模型权重或托管 Context-IR 内部实现。

## 安装

需要 Go 1.25 或更新版本。

```sh
go get github.com/ly95/h3-context-ir-go@main
```

项目从 `github.com/ly95/contextir` 迁移至上述 module path，根 Go 包名仍为 `contextir`。现有 `Document` 版本与 Schema ID 保留原标识，以兼容已保存的文档和模型回放。

新 module path 的首个版本发布前使用 `@main`。历史标签 `v0.1.0`、`v0.2.0` 仍声明旧 module path，保留原样。

## 试运行 demo

先运行一个无需模型或凭据的离线示例：

```sh
go run ./cmd/h3-context-ir-go demo
```

它演示文本需求 → `Document` → H3 prompt，输出包含完整 `result` 的 JSON。默认模式明确标记为 `draft`，不进行模型理解；也可用 `make demo` 运行同一个示例。

配置兼容端点和模型后，用 `brief` 模式编译自己的需求：

```sh
go run ./cmd/h3-context-ir-go demo \
  -mode brief \
  -base-url "$MODEL_BASE_URL" \
  -model "$MODEL_ID" \
  -key-env MODEL_API_KEY \
  -prompt "暖色晨光中，一位面包师推开木质百叶窗，把一盘面包放在柜台上。" \
  -seconds 8
```

默认协议是 Responses；Chat Completions 端点显式添加 `-api chat_completions`。命令只读取 `-key-env` 指定的凭据变量，无认证端点可省略它。`brief` 失败不会改成草稿；`needs_input` 保留结构化澄清问题，所有已发生的调用保留在 `result.calls` 中。运行 `demo -h` 查看选项，完整参数和媒体接入见 [CLI 文档](cmd/h3-context-ir-go/README.md)。

## 从需求编译

向 `compiler.New` 注入实现 `planner.Model` 的模型。素材观察可通过 `planner.Perceiver` 提供，也可使用调用方已取得的结构化 Evidence；纯文本请求的 perceiver 可以为 `nil`。

```go
package video

import (
    "context"

    "github.com/ly95/h3-context-ir-go/compiler"
    "github.com/ly95/h3-context-ir-go/h3"
    "github.com/ly95/h3-context-ir-go/planner"
)

func Compile(ctx context.Context, model planner.Model) (compiler.Result, error) {
    c, err := compiler.New(model, nil, compiler.Options{
        Profile: h3.OfficialProfile(),
        FramePolicy: compiler.FramePolicyExact,
        MaxStructureRetries: 1,
        Planner: planner.Options{MaxRetries: 1},
    })
    if err != nil {
        return compiler.Result{}, err
    }
    return c.Compile(ctx, compiler.Brief{
        Intent: "A lighthouse keeper climbs the tower stairs in a storm and lights the lamp.",
        Seconds: 15,
        Aspect: "16:9",
    })
}
```

成功结果的 `Document` 是持久化和编辑入口，`Artifact` 包含最终 prompt 与 manifest。调用方必须检查错误和 `Status`：`needs_input` 表示需要展示结构化澄清问题，尚未发布产物。无论成功或失败，均应保留返回的 `Calls`，其中包含已经发生的模型与感知用量。

可选 [openai](openai/README.md) 适配器通过官方 OpenAI Go SDK 调用 Responses 或 Chat Completions 协议，支持调用方显式配置的兼容服务；[chatcompat](chatcompat/README.md) 保留本地 Chat Completions 服务的发现与结构化输出选项，同样使用该 SDK。端点、模型、凭据、输出上限、协议和图片能力由调用方提供，库不读取环境变量、推断模型能力、切换协议或自行寻找凭据。通用图片观察合同位于 [perception](perception/README.md)。完整高层 API 见 [compiler](compiler/README.md)。

```go
model, err := openai.New(openai.Config{
    BaseURL: "https://your-compatible-service.example/v1",
    APIKey: apiKey,
    API: openai.Responses, // 或 openai.ChatCompletions，按端点能力显式选择
    Model: modelID,
    MaxOutputTokens: 8192,
    SupportsImages: true, // 仅在调用方已确认该模型支持图片时声明
})
```

此示例导入 `github.com/ly95/h3-context-ir-go/openai`；检查 `err` 后把 `model` 注入编译器。省略 `ReasoningEffort` 表示不发送该参数；省略 `APIKey` 用于无认证端点，SDK 的环境变量默认值不会启用。原 `deepseek` 适配包由通用 `openai` 包替代；原图片与观察类型迁至 `perception`，业务合同与已保存文档保持原版本。

## 能力

- 五种生成模式：`t2va`、`i2va`、`l2va`、`fl2va`、`ref2va`。
- 十六种素材职责，分别管理主体、环境、风格、分镜、结构、首尾帧、编辑、续写、植入、替换及音频引用。
- 固定与生成对白、旁白、歌唱、说话人、跨镜连续分片、可见文字与原音轨歌词 cue。固定文字保留原文，不由写作模型重新抄写。
- 内容许可、参考属性优先级、音轨复制与声音属性参考、明确的替换目标和多来源身份属性投影。
- 镜头与切点锁定、摄影机闭集、四档创意程度、内置与自定义导演，以及 LoRA 注册与约束。
- 严格 JSON、版本化 Schema、结构化诊断、有限模型修复、用量账本和确定性产物哈希。

用户意图通过显式类型或模型的结构化合同解释。代码管理时间、引用、来源、固定文字与执行约束；输入不合法、能力不足或模型输出不合约时返回明确错误。`Draft` 是单独的离线能力；失败后生成草稿必须显式选择相应政策，并保留降级状态和原因。

## 包

| 包 | 用途 |
| --- | --- |
| 根包 `contextir` | `Document`、Evidence、严格编解码、Schema、验证、错误与用量 |
| [compiler](compiler/README.md)、[planner](planner/README.md) | Brief 编译与修订、素材观察、类型化解释与规划 |
| [h3](h3/README.md)、[h3check](h3check/README.md) | H3 渲染、manifest、执行 profile 与最终文本检查 |
| [plan](plan/README.md)、[budget](budget/README.md)、[tokens](tokens/README.md) | 确定性结构、帧与画布预算、离线精确 BPE 计数 |
| [media](media/README.md) | 本地媒体检查、ffprobe/ffmpeg、视频抽帧、可选缓存与 ASR 接口 |
| [openai](openai/README.md)、[chatcompat](chatcompat/README.md) | 使用官方 OpenAI Go SDK、显式配置协议与能力的模型适配器 |
| [perception](perception/README.md) | 与模型服务无关的图片输入、观察合同与感知器 |
| [evaluate](evaluate/README.md) | 离线评分、基线与回归比较 |
| [service](service/README.md)、`uploads` | 可选 HTTP handler、记录接口与内容寻址上传存储 |
| [comfy](comfy/README.md)、[lora](lora/README.md) | ComfyUI 远端操作、LoRA 注册与运行约束 |

## 离线使用与测试

已有 `Document` 时，调用 `h3.Compile(document, profile)` 即可生成产物，不调用模型。`OfficialProfile` 检查公开格式；`ComfyUIProfile` 额外检查执行后端的帧网格、画布、socket 与输入限制。IR 使用整数毫秒和有理数 FPS，不隐式调整时长。需要 `17k+5` 对齐时，调用方显式选择 `FramePolicySnapUp`，结果同时报告请求时长与有效时长。

```sh
go run ./cmd/h3-context-ir-go schema
go run ./cmd/h3-context-ir-go contract
go run ./cmd/h3-context-ir-go validate < document.json
go run ./cmd/h3-context-ir-go compile -profile comfyui < document.json

make check
```

`make check` 执行格式检查、vet、全部原生测试、race 和解码 fuzz；需要 ffmpeg、ffprobe，以及 race detector 所需的 C 编译器。缺少媒体工具时，完整检查会先报错。也可分别运行 `go test ./...`、`go test -race ./...` 与 `go vet ./...`，但直接运行 Go 测试可能在缺少媒体工具时跳过对应集成用例。

普通 Go 测试不调用付费模型。命令行参数见 [CLI 文档](cmd/h3-context-ir-go/README.md)；真实模型录制单独使用 [live-eval](cmd/h3-context-ir-go-live-eval/README.md)，会访问显式配置的服务并消耗额度。

上游来源、逐项测试映射和 Go 适配原则见 [测试说明](docs/UPSTREAM_TESTS.md) 与 [conformance](conformance/README.md)。

## 能力边界

素材的文字说明不是观察事实。静态图片可声明完整视觉覆盖；抽样视频只覆盖实际采样帧，ASR 只建立转写词语，不能证明音色、音乐或节奏。部分覆盖需要调用方显式接纳。感知与媒体准备的具体边界见 [media](media/README.md)。

`MediaPreparation` 描述后端所需的素材准备，不表示已经裁剪、转码或生成视频。实际 H3 推理由调用方执行。IR、prompt 与单元测试的通过不能证明成片的人物一致性、音画同步、文字保真或与 MiniMax 托管服务质量等价。已知格式与执行限制见 [实现范围](docs/READINESS.md)。

## 许可证与来源

H3 Context-IR Go 使用 [Apache License 2.0](LICENSE)。OpenH3-IR、Qwen2.5 词表及测试素材的原有声明保留在 [第三方说明](THIRD_PARTY_NOTICES.md) 与 `third_party/`。模型权重、托管服务和调用方媒体的许可独立于本库。来源版本见 [UPSTREAM.md](docs/UPSTREAM.md)，贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)。
