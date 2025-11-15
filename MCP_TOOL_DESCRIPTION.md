# TTS (Text-to-Speech) MCP Tool

A local text-to-speech service that fetches/caches audio from Azure Cognitive Services and plays it through your speakers.

## Available Tools

### `play_tts`
Converts text to speech and plays it immediately.
- **Parameters**: `text` (required), `language_code` (optional, default: "en-US")
- **Use when**: User wants to hear text spoken aloud
- **Examples**:
  - Play pronunciation: "How do you pronounce 'bonjour'?"
  - Read content: "Read this paragraph to me"
  - Language practice: Play Spanish phrase with `language_code: "es-ES"`

### `fetch_tts`
Converts text to speech and caches it without playing.
- **Parameters**: `text` (required), `language_code` (optional, default: "en-US")
- **Use when**: Pre-caching audio for later use
- **Example**: "Prepare the audio for this phrase but don't play it yet"

## Supported Languages

20+ languages including: English (en-US), Spanish (es-ES, es-MX), French (fr-FR), German (de-DE), Japanese (ja-JP), Chinese (zh-CN), Korean (ko-KR), Italian (it-IT), Portuguese (pt-BR), Russian (ru-RU), Arabic (ar-SA), Hindi (hi-IN), and more.

For short codes (e.g., "fr"), defaults to most common regional variant (e.g., "fr-FR").

## Key Features

- **Cached responses**: Same text/language returns instantly from cache
- **High-quality audio**: Uses Azure Cognitive Services neural voices
- **Client-side playback**: Each client plays audio independently (multiple can play simultaneously)
- **Offline-capable**: Cached audio works without internet
- **Daemon handles fetch/cache**: Centralized caching, clients handle playback

## Best Practices

- Use for: pronunciation help, reading text aloud, language learning
- Avoid: very long texts (>1000 characters), rapid repeated requests
- Language codes: Use full codes like "es-MX" for specific accents, short codes like "es" for defaults
