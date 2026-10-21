# Explicit local media adapters

`Tools` inspects real files through explicitly configured ffprobe/ffmpeg. `Perceiver`
passes real still pixels or decoded video frames to an image-capable model. Video
coverage remains partial; an image sequence never proves unheard audio or motion
between samples. Metered ASR is a separate, explicitly supplied adapter.

`FrameCache` stores decoded sample artifacts. `AssetCardCache` separately stores
visual observation cards/evidence. Both require an absolute caller-owned directory;
neither is enabled implicitly. Observation caching also requires a positive explicit
entry byte limit and `PerceiverOptions.CacheModelIdentity`, identifying the non-secret
provider/model/revision and decoding policy. Update that identity when any of those
choices changes. Never put credentials in an identity.

```go
cards, err := media.NewAssetCardCache(media.CardCacheOptions{
    Directory: "/absolute/caller-owned/cache/cards",
    MaxEntryBytes: 8 << 20,
})
// Check err before use. Model and resolver are caller-owned adapters.
perceiver, err := media.NewPerceiver(media.PerceiverOptions{
    Model: visionModel,
    Resolve: resolveLocalMedia,
    CardCache: cards,
    CacheModelIdentity: "provider/model/revision/decoding-policy-v1",
    OnCache: observeCacheEvent,
})
```

The observation key includes the actual file SHA-256, asset identity and modality,
analyzer version, declared model policy, and a hash of the complete production
prompt/schema/structured input. Image perception constructs its request through
the same `deepseek.ImagePerceptionRequest` used for cache identity. Video sampling
timestamps and measured metadata are part of the request. A supplied asset digest
that disagrees with the bytes is rejected. Source changes during an observation,
corrupt entries and oversized entries are explicit errors. Writes are atomic;
an actual absent entry is a reported miss that requires a new observation.

Production hits require validated evidence as well as a card. The lower-level
cache also supports explicitly imported cards without evidence; those cannot
silently replace production perception. `OnCache` reports hit/bypass metadata.
Cache hits make no model call and do not replay the earlier model's usage ledger.
The optional frame cache can avoid repeated video decoding; the card cache does
not implicitly replace the caller's probing/sampling policy.

Audio metadata is never read or written by the card cache. `AudioMetadataCard`
rebuilds the caller's role/note/transcript/language/duration declarations on every
request and leaves unheard timbre/music fields empty. Direct audio cache access
returns an explicit `Bypassed` result with reason
`audio_metadata_is_request_specific`. Video caching stores only the visual result;
each request still runs its own configured ASR and retains that invocation's usage.

Run `go test ./media` to verify the native cache and metadata APIs, including the
upstream audio-cache invariants, real image hits, contract/model/content
invalidation, corrupted entries, poisoned audio entries and a real audiovisual
clip whose cached vision never reuses ASR output. The integration tests need
ffmpeg/ffprobe and report explicit skips when those tools are unavailable.
