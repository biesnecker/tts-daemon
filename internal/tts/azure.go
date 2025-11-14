package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/time/rate"
)

// AzureClient wraps the Azure Speech REST API
type AzureClient struct {
	subscriptionKey string
	region          string
	rateLimiter     *rate.Limiter
	httpClient      *http.Client
	customVoices    map[string]string // Custom voice mappings
}

// NewAzureClient creates a new Azure TTS client with rate limiting
func NewAzureClient(subscriptionKey, region string, maxQPS float64, customVoices map[string]string) *AzureClient {
	// Create rate limiter: allows maxQPS requests per second with burst of 1
	limiter := rate.NewLimiter(rate.Limit(maxQPS), 1)

	return &AzureClient{
		subscriptionKey: subscriptionKey,
		region:          region,
		rateLimiter:     limiter,
		httpClient:      &http.Client{},
		customVoices:    customVoices,
	}
}

// SynthesizeToMP3 synthesizes text to speech and returns MP3 audio data
func (a *AzureClient) SynthesizeToMP3(text, languageCode string) ([]byte, error) {
	// Wait for rate limiter before making API call
	ctx := context.Background()
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	// Get voice name for language
	voiceName := a.getVoiceNameForLanguage(languageCode)

	// Build SSML request
	ssml := fmt.Sprintf(`<speak version='1.0' xml:lang='%s'>
		<voice xml:lang='%s' name='%s'>%s</voice>
	</speak>`, languageCode, languageCode, voiceName, escapeXML(text))

	// Build request URL
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", a.region)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(ssml))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Ocp-Apim-Subscription-Key", a.subscriptionKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-16khz-128kbitrate-mono-mp3")
	req.Header.Set("User-Agent", "tts-daemon/1.0")

	// Make request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Azure API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(audioData) == 0 {
		return nil, fmt.Errorf("synthesis produced no audio data")
	}

	return audioData, nil
}

// escapeXML escapes special XML characters in text
func escapeXML(text string) string {
	// Simple XML escaping
	replacer := map[rune]string{
		'&':  "&amp;",
		'<':  "&lt;",
		'>':  "&gt;",
		'"':  "&quot;",
		'\'': "&apos;",
	}

	var result bytes.Buffer
	for _, char := range text {
		if replacement, ok := replacer[char]; ok {
			result.WriteString(replacement)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// getVoiceNameForLanguage maps language codes to Azure voice names
func (a *AzureClient) getVoiceNameForLanguage(languageCode string) string {
	// Check custom voice mapping first - exact match
	if a.customVoices != nil {
		if voice, ok := a.customVoices[languageCode]; ok {
			return voice
		}

		// If no exact match, try base language (e.g., "es" for "es-MX")
		if len(languageCode) > 2 && languageCode[2] == '-' {
			baseLanguage := languageCode[:2]
			if voice, ok := a.customVoices[baseLanguage]; ok {
				return voice
			}
		}
	}

	// Fall back to defaults
	// Map of language codes to recommended neural voices
	voiceMap := map[string]string{
		"en":    "en-US-JennyNeural",
		"en-US": "en-US-JennyNeural",
		"en-GB": "en-GB-SoniaNeural",
		"en-AU": "en-AU-NatashaNeural",
		"fr":    "fr-FR-DeniseNeural",
		"fr-FR": "fr-FR-DeniseNeural",
		"fr-CA": "fr-CA-SylvieNeural",
		"es":    "es-ES-ElviraNeural",
		"es-ES": "es-ES-ElviraNeural",
		"es-MX": "es-MX-DaliaNeural",
		"de":    "de-DE-KatjaNeural",
		"de-DE": "de-DE-KatjaNeural",
		"it":    "it-IT-ElsaNeural",
		"it-IT": "it-IT-ElsaNeural",
		"pt":    "pt-BR-FranciscaNeural",
		"pt-BR": "pt-BR-FranciscaNeural",
		"pt-PT": "pt-PT-RaquelNeural",
		"ja":    "ja-JP-NanamiNeural",
		"ja-JP": "ja-JP-NanamiNeural",
		"ko":    "ko-KR-SunHiNeural",
		"ko-KR": "ko-KR-SunHiNeural",
		"zh":    "zh-CN-XiaoxiaoNeural",
		"zh-CN": "zh-CN-XiaoxiaoNeural",
		"zh-TW": "zh-TW-HsiaoChenNeural",
		"ru":    "ru-RU-SvetlanaNeural",
		"ru-RU": "ru-RU-SvetlanaNeural",
		"ar":    "ar-SA-ZariyahNeural",
		"ar-SA": "ar-SA-ZariyahNeural",
		"hi":    "hi-IN-SwaraNeural",
		"hi-IN": "hi-IN-SwaraNeural",
	}

	if voice, ok := voiceMap[languageCode]; ok {
		return voice
	}

	// Default to US English if language not found
	return "en-US-JennyNeural"
}
