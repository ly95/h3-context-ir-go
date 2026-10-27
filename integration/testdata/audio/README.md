# Pinned real-speech audio fixture

`jfk.flac` contains the unchanged 11-second real-speech fixture from OpenAI
Whisper at revision `86098128c0b4f24f0e2aa2994de830614b474227`. Its original
repository license is retained in `WHISPER_LICENSE.txt`. Retaining that repository
license does not add a separate file-specific rights assertion.

`manifest.json` records the immutable download URLs, byte counts, SHA-256 digests,
and independently measured FLAC properties: 44,100 Hz, stereo, 24-bit samples.
The exact-word assertion comes from SYSTRAN faster-whisper's unbatched
`test_transcribe` at revision `9e472ae9de8bb374552f531f752654554b76b2d6`;
`FASTER_WHISPER_LICENSE.txt` preserves that reference source's MIT license.
Neither source test is executed by the Go library.

The expected transcript is a held-out assertion. Do not send this manifest, the
reference test, expected words, speaker identity or transcript hints to an ASR or
native audio-perception request. Compare the actual recognition result afterward
and preserve it unchanged. Record exact punctuation separately from word-sequence
accuracy; the upstream reference includes a decoder-leading space that is omitted
only from the manifest's assertion text.

Physical metadata and recognized words do not establish voice identity, gender,
age, accent, timbre, music, rhythm or sound texture. The fixture is not an observed
Evidence object and does not prove that any provider has perceived audio.
Compiling a prompt that references it does not verify speech generation, voice
cloning, pronunciation, synchronization or generated-video quality.
