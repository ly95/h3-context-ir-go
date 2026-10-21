# ContextIR

ContextIR 是独立的 Go 视频提示词编译库。它把自然语言需求与素材观察编译为可编辑、可持久化的 IR，再从同一份 `Document` 生成 H3 prompt、素材绑定、预算和内容哈希。

库与测试使用 Go 标准库，不需要 Python。模型、感知、存储和 HTTP 执行能力由调用方显式配置；核心验证与渲染可以完全离线运行。

本项目参考 [OpenH3-IR](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5) 与 MiniMax 公开的 H3 格式指南，是独立、非官方实现。它不包含 MiniMax 的模型权重或托管 Context-IR 内部实现。

## 安装

需要 Go 1.25 或更新版本。

```sh
go get github.com/ly95/contextir@latest
```

## 从需求编译

向 `compiler.New` 注入实现 `planner.Model` 的模型。素材观察可通过 `planner.Perceiver` 提供，也可使用调用方已取得的结构化 Evidence；纯文本请求的 perceiver 可以为 `nil`。

```go
package video

import (
    "context"

    "github.com/ly95/contextir/compiler"
    "github.com/ly95/contextir/h3"
    "github.com/ly95/contextir/planner"
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

内置 [deepseek](deepseek/README.md) 和 [chatcompat](chatcompat/README.md) 提供模型适配器。端点、模型、凭据、输出上限与推理选项均由调用方提供，库不会读取环境变量、切换模型或自行寻找凭据。完整高层 API 见 [compiler](compiler/README.md)。

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
| [deepseek](deepseek/README.md)、[chatcompat](chatcompat/README.md) | 显式配置的模型与图片请求适配器 |
| [evaluate](evaluate/README.md) | 离线评分、基线与回归比较 |
| [service](service/README.md)、`uploads` | 可选 HTTP handler、记录接口与内容寻址上传存储 |
| [comfy](comfy/README.md)、[lora](lora/README.md) | ComfyUI 远端操作、LoRA 注册与运行约束 |

## 离线使用与测试

已有 `Document` 时，调用 `h3.Compile(document, profile)` 即可生成产物，不调用模型。`OfficialProfile` 检查公开格式；`ComfyUIProfile` 额外检查执行后端的帧网格、画布、socket 与输入限制。IR 使用整数毫秒和有理数 FPS，不隐式调整时长。需要 `17k+5` 对齐时，调用方显式选择 `FramePolicySnapUp`，结果同时报告请求时长与有效时长。

```sh
go run ./cmd/contextir schema
go run ./cmd/contextir contract
go run ./cmd/contextir validate < document.json
go run ./cmd/contextir compile -profile comfyui < document.json

make check
```

`make check` 执行格式检查、vet、全部原生测试、race 和解码 fuzz；需要 ffmpeg、ffprobe，以及 race detector 所需的 C 编译器。缺少媒体工具时，完整检查会先报错。也可分别运行 `go test ./...`、`go test -race ./...` 与 `go vet ./...`，但直接运行 Go 测试可能在缺少媒体工具时跳过对应集成用例。

普通 Go 测试不调用付费模型。命令行参数见 [CLI 文档](cmd/contextir/README.md)；真实模型录制单独使用 [live-eval](cmd/contextir-live-eval/README.md)，会访问显式配置的服务并消耗额度。

上游来源、逐项测试映射和 Go 适配原则见 [测试说明](docs/UPSTREAM_TESTS.md) 与 [conformance](conformance/README.md)。

## 能力边界

素材的文字说明不是观察事实。静态图片可声明完整视觉覆盖；抽样视频只覆盖实际采样帧，ASR 只建立转写词语，不能证明音色、音乐或节奏。部分覆盖需要调用方显式接纳。感知与媒体准备的具体边界见 [media](media/README.md)。

`MediaPreparation` 描述后端所需的素材准备，不表示已经裁剪、转码或生成视频。实际 H3 推理由调用方执行。IR、prompt 与单元测试的通过不能证明成片的人物一致性、音画同步、文字保真或与 MiniMax 托管服务质量等价。已知格式与执行限制见 [实现范围](docs/READINESS.md)。

## 许可证与来源

ContextIR 使用 [Apache License 2.0](LICENSE)。OpenH3-IR、Qwen2.5 词表及测试素材的原有声明保留在 [第三方说明](THIRD_PARTY_NOTICES.md) 与 `third_party/`。模型权重、托管服务和调用方媒体的许可独立于本库。来源版本见 [UPSTREAM.md](docs/UPSTREAM.md)，贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)。
