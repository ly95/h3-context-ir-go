# Explicit image perception

`ImagePerceiver` turns actual image inputs into validated observation evidence.
The caller supplies an `ImageModel` and an `ImageResolver`; the package does not
select a provider, read credentials, open media files, or fetch image locations.
Model adapters such as `openai.Client` implement the image-model contract.

`PerceptionOptions.MaxRetries` explicitly bounds content repair. Every actual
model attempt retains its usage and invokes `OnCall`, including rejected output
and model errors. Invalid or unavailable usage, cancellation, and transport
failures stop further repair. Use `PerceiveWithCalls` for the complete call ledger.

`ImagePerceptionRequest` supplies the same versioned prompt, schema, and structured
input used by production observation and cache identity. The migration from the
former `deepseek` package preserves their exact contents and versions. Error
diagnostics retain the historical `deepseek.image_perception` stage so callers
can continue recognizing the same observation failures; the stage does not
select or require a model provider.
