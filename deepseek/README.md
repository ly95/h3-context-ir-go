# DeepSeek adapter

This package implements `planner.Model` with DeepSeek's [Responses API](https://api-docs.deepseek.com/api/create-response/). It uses Go's standard library and sends requests directly to the caller's explicit API root. It does not use an OpenAI SDK or service, load credentials, download images, or retry HTTP calls.

```go
client, err := deepseek.New(deepseek.Config{
    BaseURL:         "https://api.deepseek.com",
    APIKey:          callerSuppliedSecret,
    Model:           "deepseek-flash",
    MaxOutputTokens: 8192,
    ReasoningEffort: "none",
})
```

`Generate(ctx, request)` sends `request.System` as `instructions`, the complete JSON input as the user `input` string, and the complete supplied schema under `text.format` with `type: "json_schema"`. No schema fields are removed or converted into plain JSON mode. The selected model and output-token limit are checked against the response.

`GenerateWithImages(ctx, request, []deepseek.Image)` uses an explicit user message with `input_text` and `input_image` parts. Only `deepseek-flash` supports images. `Image.URI` is supplied by the caller; supported locators are HTTPS, loopback HTTP, and base64 data URLs for JPEG, PNG, GIF, or WebP. `Image.Detail` may be empty, `auto`, `low`, `high`, or `original`. The adapter validates the locator but does not inspect or fetch the image itself. See DeepSeek's [vision documentation](https://api-docs.deepseek.com/guides/vision/).

Timeout defaults to a positive timeout on the supplied HTTP client, or two minutes. `Config.Timeout` overrides it explicitly. Responses default to an 8 MiB limit; oversized bodies fail rather than being truncated. The full encoded request is limited to DeepSeek's 48 MiB maximum. A custom `HTTPClient` is copied and redirects are disabled; its other fields remain available for explicit proxy or transport configuration. The default transport does not read proxy environment variables. Plain HTTP API roots require loopback addresses.

Every response must complete successfully, contain one assistant message with valid JSON, and report both nonnegative token counts. Missing usage fields are returned as `-1`, never as an assumed zero. Already parsed model and usage metadata are retained on failures, including incomplete output, refusal, HTTP errors, or a hard-limit overage. Rejected counts must not be charged. Errors contain stable diagnostics and HTTP status codes without provider bodies, private locators, or API keys. Context cancellation and deadlines remain inspectable with `errors.Is`.

Tests use local `httptest` servers. Passing them verifies this adapter's wire contract and failure handling; it does not establish generation quality or equivalence to official H3 Context-IR.
