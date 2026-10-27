# 原生视频／音频理解与完整 Compile 验收

`TestLiveNativeMediaCompile` 通过显式注入的测试适配器，把真实媒体交给
Gemini 原生接口感知，再执行当前库的完整 `Compiler.Compile`。此适配器只用于
验收，不是已发布的生产适配器，也不会给 core 引入模型供应商依赖。

## 运行

需要 Go、可执行的 `ffmpeg` 和 `ffprobe`，以及可用的 OpenCode Go 模型权限。
测试包含真实付费请求；普通 `go test ./...` 默认跳过。

先通过本机环境或私有配置设置 `CONTEXTIR_NATIVE_MEDIA_API_KEY`。不要把密钥
写入源码、README、命令行参数、截图或日志。随后从仓库根目录执行：

```sh
CONTEXTIR_NATIVE_MEDIA_LIVE=1 \
CONTEXTIR_NATIVE_MEDIA_BASE_URL=https://opencode.ai/inference/go/google/v1beta \
CONTEXTIR_NATIVE_MEDIA_MODEL=gemini-4-argon \
go test ./integration -run '^TestLiveNativeMediaCompile$' -count=1 -v
```

可另设 `CONTEXTIR_NATIVE_MEDIA_OUTPUT_DIR`，把本轮结果、提示词、用量和失败诊断
保存到明确指定的目录，例如 `/tmp/contextir-native-media-run`。未设置时保存在
`.ci/native-media-live/<UTC时间>/`。固定公开样本的结果包含媒体观察和编译正文；
若将此测试扩展到私有媒体，应自行保管这些结果。

## 输入与检查范围

- 视频使用仓库原始 `compiler/testdata/remaining_media/off-vs-on.mp4`；直接发送
  MP4 字节，不在本地抽帧替代原生视频输入。
- 音频使用原始 `integration/testdata/audio/jfk.flac`；直接发送 FLAC 字节，
  不使用 ASR 中间结果。固定来源、SHA-256、许可和独立期望见
  [音频 fixture](../../integration/testdata/audio/README.md)。
- 感知请求不包含独立期望答案、字幕抄本或预设 transcript。识别后再做独立对照；
  编译阶段的精确对白保留要求来自本轮实际观察到的原词。
- 同一轮产生的原生 Evidence 立即用于完整 Compile，并把实际感知用量计入一次。
  这不是跨轮缓存或录制回放；`partial` coverage 必须显式授权并保留其限制。

Gemini 仅支持部分 JSON Schema 能力，因此测试传输采用原生 JSON 外层 envelope。
解出内层后仍执行库原有的解码、锁定字段和 core 校验，不放宽原始契约。

验收检查文档、引用绑定、精确对白、渲染产物和实际调用账本。自然首轮成功与发生
修复分别记录；失败保留本轮已发生调用和 partial result，不以反复重跑后的成功
覆盖先前失败。失败、跳过或仅收到 HTTP 200 均不能当作完整验收成功。

## 当前结果

2026-10-26 使用 OpenCode Go 的 `gemini-4-argon` 完成一轮真实运行，两项完整
Compile 验收均通过，总耗时约 213 秒，18 次实际调用，输入 92,705 tokens、输出
10,352 tokens。所有回执的计费来源均为 `go`。

| 场景 | 原始输入 | 完整链路 | 实际调用 | 耗时 |
| --- | --- | --- | --- | --- |
| 视频 | 10.144 秒 MP4 | 原生视频观察 → 编辑源引用 → Compile → 语义审查 → 渲染复核 | 11 次，含 3 次规划尝试 | 148.72 秒 |
| 音频 | 11 秒 FLAC | 原生音频识词 → 原词选择 → 声音引用绑定 → Compile → 语义审查 → 渲染复核 | 7 次 | 64.27 秒 |

原生回执分别记录 `VIDEO` 950 tokens、`AUDIO` 291 tokens。音频独立期望的 22 词
全部一致；识别标点与期望文本不同，已单独记录，最终对白逐字保留的是实际观察
到的原文，没有用期望文本替换。两项最终 Artifact 均可从权威 Document 完整重编译。

视频的分屏、环境、主要行动顺序与两段英文文字经独立核对成立；人物属性、相同
身份和具体运镜仍有未经独立证实的推断，视觉语义评定为 `partial`。模型自己的
语义审查通过不代表这些推断已被外部确认。目标切点为 5.2 秒，源切点在
4.50–4.75 秒之间，未做到精确时间保持；第二镜头的区域运镜表达也有歧义。
本轮音频范围是识词与语言，不包含
音色、音乐、环境音理解，视频源自带音轨也不计为已完成声学验收。

完整 [运行清单](docs/2026-10-26/manifest.json)、
[独立核对](docs/2026-10-26/independent-review.json)、
[视频提示词](docs/2026-10-26/video/prompt.txt)、
[音频提示词](docs/2026-10-26/audio/prompt.txt) 已入库。
`video/` 和 `audio/` 同时保留全部调用回执、候选响应、Evidence 和 Result，
包括视频前两次未接受的规划候选。普通 `go test ./...`、`go test -race ./...`、
`go vet ./...` 均通过；它们默认不重复这轮真实 API 调用。

原生传输、语义理解和完整编译需要分别报告。通过测试也不证明所有媒体内容均被
理解，不证明音色识别、声音克隆、音画同步或生成视频质量，也不代表生产发布、
官方实现等价或与未修改上游测试完全兼容。
