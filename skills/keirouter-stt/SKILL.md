---
name: keirouter-stt
description: Speech-to-text via KeiRouter batch (/v1/audio/transcriptions) and realtime WebSocket (/v1/audio/transcriptions/stream) using OpenAI Whisper / Groq / Gemini / Deepgram / AssemblyAI / xAI models. Use when the user wants to transcribe audio, convert speech to text, stream live dictation, or get subtitles from audio files.
---

# KeiRouter — Speech-to-Text

Requires `KEIROUTER_URL` (and `KEIROUTER_KEY` if auth enabled). See https://raw.githubusercontent.com/mydisha/keirouter/main/skills/keirouter/SKILL.md for setup.

## Discover

```bash
curl $KEIROUTER_URL/v1/models/stt | jq '.data[].id'
# Per-model params (language, response_format, prompt, temperature support)
curl "$KEIROUTER_URL/v1/models/info?id=openai/whisper-1"
```

`model` = STT model ID (e.g. `openai/whisper-1`, `groq/whisper-large-v3`, `deepgram/nova-3`, `gemini/gemini-2.5-flash`, `xai/grok-stt`).

## Batch endpoint

`POST $KEIROUTER_URL/v1/audio/transcriptions` (OpenAI Whisper compatible, `multipart/form-data`)

| Field | Required | Notes |
|---|---|---|
| `model` | yes | from `/v1/models/stt` |
| `file` | yes | audio file (mp3, wav, m4a, webm, ogg, flac) |
| `language` | no | ISO-639-1 (e.g. `en`, `vi`) |
| `prompt` | no | hint text to guide transcription (xAI: mapped to `keyterm`) |
| `response_format` | no | `json` (default) / `text` / `verbose_json` / `srt` / `vtt` |
| `temperature` | no | 0–1 |

```bash
curl -X POST "$KEIROUTER_URL/v1/audio/transcriptions" \
  -H "Authorization: Bearer $KEIROUTER_KEY" \
  -F "model=xai/grok-stt" \
  -F "file=@audio.mp3" \
  -F "language=en"
```

## Realtime WebSocket (streaming STT)

`GET $KEIROUTER_URL/v1/audio/transcriptions/stream` → WebSocket upgrade.

Currently supported for **xAI** (`xai/grok-stt`). Proxies bidirectionally to `wss://api.x.ai/v1/stt`.

| Query | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `xai/grok-stt` |
| `sample_rate` | no | default upstream 16000 |
| `encoding` | no | `pcm` (default), `mulaw`, `alaw` |
| `interim_results` | no | `true` for partials ~every 500ms |
| `language` | no | enables ITN formatting |
| `keyterm` | no | bias term (repeatable); `prompt` also maps to keyterm |
| `diarize`, `multichannel`, `channels`, `filler_words`, `endpointing`, `smart_turn`, `smart_turn_timeout` | no | forwarded upstream |

Auth: same as other `/v1` routes (`Authorization: Bearer $KEIROUTER_KEY` or `x-api-key` on the handshake).

### Protocol (xAI-compatible)

1. Wait for `{"type":"transcript.created"}`
2. Send raw PCM as **binary** WebSocket frames (~100ms chunks)
3. Optional control text: `{"type":"finalize"}` (PTT), `{"type":"audio.done"}` (end)
4. Receive `transcript.partial` (`is_final` / `speech_final`) and final `transcript.done`

```js
import WebSocket from "ws";

const url = new URL(`${process.env.KEIROUTER_URL}/v1/audio/transcriptions/stream`);
url.searchParams.set("model", "xai/grok-stt");
url.searchParams.set("sample_rate", "16000");
url.searchParams.set("encoding", "pcm");
url.searchParams.set("interim_results", "true");
url.searchParams.set("language", "en");

const ws = new WebSocket(url, {
  headers: { Authorization: `Bearer ${process.env.KEIROUTER_KEY}` },
});

ws.on("message", (data, isBinary) => {
  if (isBinary) return;
  const ev = JSON.parse(data.toString());
  if (ev.type === "transcript.created") {
    // start sending PCM binary frames…
  } else if (ev.type === "transcript.partial") {
    console.log(ev.is_final ? "FINAL" : "partial", ev.text);
  } else if (ev.type === "transcript.done") {
    console.log("done", ev.text, ev.duration);
    ws.close();
  }
});

// When finished speaking:
// ws.send(JSON.stringify({ type: "audio.done" }));
```

## Response shape (batch)

Default (`response_format=json`):
```json
{ "text": "Hello, this is the transcription." }
```

`verbose_json` adds `language`, `duration`, `segments[]` with timestamps.
`srt` / `vtt` return subtitle text.

## Provider quick reference

| Provider | `model` format | Batch | Realtime WS |
|---|---|---|---|
| OpenAI | `whisper-1`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe` | yes | no |
| Groq | `whisper-large-v3`, `whisper-large-v3-turbo` | yes | no |
| Gemini | `gemini-2.5-flash`, `gemini-2.5-pro` | yes | no |
| Deepgram | `nova-3`, `nova-2` | yes | no |
| AssemblyAI | `universal-3-pro`, `universal-2` | yes | no |
| xAI | `grok-stt` | yes (`POST /v1/stt`) | yes (`GET /v1/audio/transcriptions/stream`) |

## Accounts

xAI STT reuses the same provider accounts as chat/image under **AI Providers → xAI (Grok)**. Connect a key once; it is available for LLM, image, search, batch STT, and realtime STT.

## Notes (xAI)

- Upstream has no model parameter; KeiRouter routes via `xai/grok-stt`.
- Batch: `language` enables Inverse Text Normalization (`format=true`); `prompt` → `keyterm`.
- Realtime: PCM16 @ 16 kHz recommended; wait for `transcript.created` before sending audio.
- Realtime does **not** support HTTP relay proxies (CONNECT HTTP/SOCKS proxies still work).
