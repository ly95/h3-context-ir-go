# ContextIR

独立的 Go package：把自然语言视频需求和真实素材观察编译成可编辑、可持久化的 IR，再从同一份 IR 生成 H3 prompt、输入绑定、预算和内容哈希。核心及适配器只使用 Go 标准库，运行时不依赖 Python、CLW Studio、数据库或队列。

验收依据是 [OpenH3-IR 固定提交 fd031e1](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5) 的完整功能与原始测试。原测试输入、原断言、历次失败和真实 DeepSeek 输出分别保存在 [conformance](conformance/README.md)，未把早期三个自编 smoke cases 当作功能验收。具体通过范围和剩余差异见 [验收清单](docs/UPSTREAM_TESTS.md) 与 [当前状态](docs/READINESS.md)。

最终逐项验收覆盖原清单 **991/991 项，全部通过**：672 项由未修改的原断言验证实际 Go 主输出，319 项由明确记录接口/输入调整的独立 Go 测试验证；两项语言兼容要求已按 Go 实现，N/A 为 0。没有遗漏、重复计数或跳过来凑通过。原样 Python 桥接重跑仍保留 **981 passed / 10 failed**，没有改写成 991 passed。见 [完整逐项报告](conformance/reports/native-functionality-go-compatibility-v1.json) 与 [差异说明](docs/UPSTREAM_TESTS.md)。

真实最低 Go 1.25.0 的全库测试与 vet 已通过，见 [兼容性证据](conformance/reports/go-minimum-compatibility-v1.json)。旧 989 项通过、2 项 N/A 的报告作为历史保留。

这是公开协议的独立实现，没有 MiniMax 托管 Context-IR 的内部代码、训练数据或生成质量等价证据。

## 安装

需要 Go 1.25+。私有仓库调用方需要相应 GitHub 访问权限，可按环境设置 `GOPRIVATE=github.com/ly95/*`，不要把凭据放进 module URL。

```sh
go get github.com/ly95/contextir@v0.1.1-0.20261021024005-84c82a35fe9f
```

当前版本固定产品提交 `84c82a35fe9f84b9179522f3925ff1d1d7e15324`，已从 GitHub 私有远端下载验证，ZIP 为 9,385,184 字节。独立消费者构建、IR/H3/哈希/评分重放，以及下载包内完整 Go 测试 **372 个主测试、7813 个子测试全部通过，0 失败、0 跳过**。没有 `replace`、工作区源码或历史报告依赖；下载源码逐项匹配真实 Go 1.25.0 最低版本测试的输入。见 [消费者验证](conformance/reports/private-consumer-go-compatibility-v4.json)、[下载包完整测试](conformance/reports/private-native-go-compatibility-v1.json) 与 [汇总证据](conformance/reports/completion-go-compatibility-v1.json)。

当前实现与证据位于 `codex/readiness-audit` 分支。此前 `874c5ee` 的 989/2 N/A 验收和分发记录作为历史保留；旧标签 `v0.1.0` 不包含后续实现。

历史模型输出、源码快照和测试报告保存在独立的审计 module `conformance/reports`。安装 Go 库不下载这些历史文件；需要完整证据时克隆 Git 仓库。

## 从一句需求编译

```go
client, err := deepseek.New(deepseek.Config{
    BaseURL: "https://api.deepseek.com",
    Model: "deepseek-flash",
    APIKey: apiKey, // 宿主显式取得；库不读环境变量或 dotenv。
    ReasoningEffort: "low",
    MaxOutputTokens: 12288,
})
if err != nil { return err }

c, err := compiler.New(client, myPerceiver, compiler.Options{
    Profile: h3.ComfyUIProfile(),
    FramePolicy: compiler.FramePolicySnapUp,
    MaxStructureRetries: 1,
    Planner: planner.Options{MaxRetries: 1},
})
if err != nil { return err }
r, err := c.Compile(ctx, compiler.Brief{
    Intent: "a lighthouse keeper climbs the tower stairs in a storm and lights the lamp",
    Seconds: 15,
    Aspect: "16:9",
})
// 无论成功与否均保留 r.Calls；已发生的用量不能随错误丢失。
if err != nil { return err }
if r.Status == "needs_input" {
    // 展示 r.ModeDecision.Questions，用户作出结构化选择后重新提交。
    return nil
}
// 持久化 r.Document，并把 r.Artifact 交给实际 H3 推理适配器。
```

导入 module 的 `compiler`、`deepseek`、`h3`、`planner` 包。纯文本请求的 `myPerceiver` 可以为 nil。`chatcompat.Client` 提供另一个可注入的 Chat Completions 文本/视觉后端，详见其 [说明](chatcompat/README.md)。

管线中的模式、素材职责、替换目标、内容许可和参考属性优先级通过独立的类型化模型合同解释；不使用关键词列表或正则猜测用户意图。代码管理镜头边界、来源与输入绑定、固定对白槽位、摄影机闭集、预算、校验和有限修复。固定对白由宿主回填原文，写作模型不负责重新抄写它。

画风、引用职责与音轨复制关系同样由宿主管理。环境、结构、植入/替换、音频属性、参考图帧等容易混淆的输入还有独立的 `semantic_review`：审核最终正文是否把场景当演员、偷用未许可内容、遗漏植入人物或改变锚定图像。明确错误进入有限修复；审核通过不表示已经证明任意语义或成片质量。审核调用与写作调用逐次进入同一用量账本。

`Compile` 默认明确失败。`Draft` 是单独的离线能力；只有显式选择 `FailurePolicyDraft` 才允许失败后生成带降级状态、原因和真实用量的草稿。

## 模式、引用与控制

- 五种模式：`t2va`、`i2va`、`l2va`、`fl2va`、`ref2va`，明确区分实际首尾帧与内容参考。
- 原项目全部 16 种素材职责；主体身份与参考姿态、参考图排版、环境、风格、视频结构分别管理。
- 视频续写、编辑、主体放入和替换；目标不唯一时不按“剩下的人”猜测。未观察到但由用户明确命名的人物保留为 `user_symbolic`，不会伪造观察事实。
- 同步音轨、独立音频、voice timbre、music style、beat reference、SFX；属性参考不会被误当成原信号复制。`AudioCopyModes` 明确选择 BGM 的完整/部分复制，默认部分复制；完整最终录音不能暗中混入新对白、配乐或另一个复制音轨。
- 固定与生成对白、说话人、旁白、歌唱、跨镜连续分片、可见文字和复用音轨歌词 cue。`TranscriptUses` 只把明确选定的转写原文用于重新表演，不因拥有转写就授权说话或复制信号；禁止把台词重复写成画面文字。
- `SubjectProjections` 通过明确来源 ID 与属性选择合并目标身份，保留观察与用户目标声明的不同来源，不把姿态、布景或不相关服装一起带入。
- 镜头数量/切点锁定、摄影机闭集、四档创意程度、七种内置导演及自定义说明；用户与参考优先于导演。
- LoRA 注册、权重哈希、checkpoint/尺寸/帧数/步数约束、强度和触发词管理。
- `Document` 是产物唯一事实来源；prompt、manifest、来源元数据、输入准备指令和哈希从它派生。

完整 Go API 见 [compiler](compiler/README.md)；低层 `plan`、`budget`、`tokens`、`h3check` 可单独使用。

[evaluate](evaluate/README.md) 提供独立离线评分、聚合、历史基线和回归比较。指标读取最终 H3 正文与真实调用记录；编译和校验失败阻断验收，篇幅、警告数与镜头差异度只作报告。旧基线没有测量的指标保持缺失，不补零参与比较。

## 素材感知与外部执行

`media` 提供显式本地解析器、文件头事实、ffprobe/ffmpeg 量测、真实视频抽帧及可选的调用方目录缓存。`deepseek.ImagePerceiver` 和 `chatcompat.Client` 接受真实像素；素材的文字 note 不是观察事实。

静态图片观察可以声明完整视觉覆盖；抽样视频明确是 `partial`，记录实际采样时间。音频可量测并接入调用方的 ASR，转写只说明词语，不能当成听懂音色、音乐和节奏。默认 Planner 不接纳部分覆盖；需要调用方显式设置 `AllowPartialPerception`。库没有偷偷用文字代替未支持的模态。

`MediaPreparation` 是绑定所需的尺寸/帧数准备指令，和原文件元数据分开；它不表示已经裁剪或转码。实际素材准备和 H3 视频生成由调用方执行。`comfy` 提供远端 ComfyUI HTTP 客户端和图修改工具，详见 [说明](comfy/README.md)。

可选 [service](service/README.md) 包提供标准库 HTTP handler、公开字段/角色/限额合同、上传、编译、读取和修订。公开 wire contract 为版本 3，如实公布实际拒绝码，并透传 `audio_copy_modes`、`transcript_uses`、`subject_projections`，拒绝未知字段或非法嵌套值。`uploads` 是独立的内容寻址、配额、在途预留、TTL/LRU 存储。核心库不自动监听网络，也不自动创建全局存储。

## 时间、预算与持久化

IR 使用整数毫秒和有理数 FPS。低层 `h3.Compile` 不改变时长；`OfficialProfile` 负责公开格式，`ComfyUIProfile` 另外检查实际运行时的帧网格、尺寸和输入限制。

高层默认 `FramePolicyExact`。示例明确选择 `FramePolicySnapUp` 来匹配原项目的 `17k+5` 网格：结果同时报告 nominal/effective duration、实际帧数与对齐策略，没有隐式取整。`budget` 计算画布与 latent rows；`tokens` 内嵌原项目的完整 Qwen2.5 词表并作精确 BPE 计数，离线运行。

`contextir.Decode` / `DecodeEvidence` 拒绝未知字段、大小写别名、重复 keys、尾随 JSON、无效 UTF-8 与不成对 surrogate。`Validate` 检查时间、跨引用、来源和请求合同。默认上限 8 MiB，超限明确报错，不截短。发布 Schema 由 Go 类型生成并受同步测试保护。

## 命令行与复验

```sh
# 确定性操作，不调用模型。
go run ./cmd/contextir schema
go run ./cmd/contextir contract
go run ./cmd/contextir directors
go run ./cmd/contextir validate < document.json
go run ./cmd/contextir compile -profile comfyui < document.json
go run ./cmd/contextir-score score -kind result -in compile-result.json -as-run -out run.json
go run ./cmd/contextir-score baseline -in run.json -out baseline.json
go run ./cmd/contextir-score compare -baseline baseline.json -candidate next-run.json

go test ./...
go test -race ./...
go vet ./...
```

全部适用原要求的复验入口是 `conformance/run_native_functionality.py --require-all --go-compatibility-report <最低版本报告>`。它同时核查独立 Go 测试和 fresh 原断言报告中的主 Go API 调用、实际 validator 调用、不可变 bridge/runtime/fixture/录音请求来源；命令和完整前置条件见 [复验说明](conformance/README.md)。普通 Go 测试不调用付费服务，最终本地质量检查见 [记录](conformance/reports/local-quality-go-compatibility-v1.json)。

高层 `brief` / `draft` 及预算、token、原始 H3 校验命令的参数见 `go run ./cmd/contextir <command> -h`。付费模型验证是单独命令，不在普通单元测试中触发：

```sh
# 凭据由当前进程环境显式提供；不读取 dotenv，不把 key 写进参数。
go run ./cmd/contextir-upstream-eval -kind suite \
  -input conformance/fixtures/suite_briefs.json -out /tmp/contextir-new-suite \
  -base-url https://api.deepseek.com -model deepseek-flash -reasoning-effort low
```

真实模型验收保存每个原输入、每次 request/schema/output、用量、失败和产物。`proposed`、`licence_decided` 或 JSON 结构有效并不表示原要求通过；另用独立 oracle 和原断言验证。混合 pytest 的 `991 passed` 也不等于 991 项都验证了 Go 主行为。本轮逐项映射区分了真实主输出、仅调用辅助函数的历史记录和独立 Go 适配；真实模型的失败、重试和警告另列，不用确定性测试通过替代语义或成片质量。

## 来源

格式依据 MiniMax 官方[基础指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_base_en.md)与[全参考指南](https://huggingface.co/MiniMaxAI/MiniMax-H3/blob/main/docs/VIDEO_PROMPT_WRITING_GUIDE_ref_en.md)。原项目测试、算法和词表来源及许可证保留在 [third_party](third_party/open-h3-ir/NOTICE) 和 [来源说明](docs/UPSTREAM.md)。
