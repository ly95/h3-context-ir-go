# H3 文本验证器

`h3check.Validate(text, context)` 是纯 Go、标准库实现，直接验证最终 H3 文本。它不调用 Python、模型或外部进程。

```go
ctx := h3check.NewContext()
ctx.Mode = "ref2va"
ctx.Pictures = &pictureCount
ctx.DeclaredRoles = []h3check.RoleBinding{
    {Label: "<Picture 1>", Role: "style"},
}
findings := h3check.Validate(prompt, ctx)
for _, finding := range findings {
    // Rule、Severity、Msg 的 JSON 名为 rule、severity、msg。
    // ERROR 阻止使用产物；WARN 和 INFO 应向调用方报告。
}
```

Context 覆盖上游的全部字段，包括素材数量、明确声明的角色、音视频配对、独立音频来源、已转录原文、固定对白、画面文字、镜头数、时长、LoRA 触发词、参考图类型和明确创意权限。JSON 反序列化支持上游 tuple 形式，并安装上游默认值。Go 调用方使用 `NewContext()` 获取同样的默认值，随后设置显式业务事实。

权限只来自 `Context.Scope` 的结构化 `level / requested / forbidden`；本包不会从用户句子的关键词、正则或语气猜测用户要求、授权、模式或角色。对已生成 H3 文本的镜头、标签、对白、词汇和污染检查保持上游规则 ID 与 ERROR/WARN/INFO 级别；粗粒度输出媒介/重复性检查保持 WARN 建议，不用于理解或路由用户输入。

运行 `go test ./h3check`。`upstream_test.go` 对固定 SHA 的 386 次原 validator 调用和 23 个命名 controls 比较规则 ID、严重度与出现次数，并保留可操作的修复说明。原生合同测试另验证错误边界与实际产物；测试不需要 Python。组件语料的一致性不能替代完整 Compile 或视频质量验证，见 [conformance](../conformance/README.md)。

本包移植自 [ruashots/open-h3-ir](https://github.com/ruashots/open-h3-ir) 的 `h3ir/validate.py`，固定提交 `fd031e136ae3d89147324c0d1b2aa65e838c21f5`，包含其 `grid.py`、`shots.py`、`style.py`、`creativity.py` 的相关常量与规则。Apache-2.0 许可证和版权声明见 [`third_party/open-h3-ir`](../third_party/open-h3-ir)。测试只能证明该公开验证合同的一致性，不能证明真实模型理解正确或 H3 成片质量。
