package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// Voice represents an Azure TTS voice from the API
type Voice struct {
	Name              string   `json:"Name"`
	DisplayName       string   `json:"DisplayName"`
	ShortName         string   `json:"ShortName"`
	Gender            string   `json:"Gender"`
	Locale            string   `json:"Locale"`
	VoiceType         string   `json:"VoiceType"`
	Status            string   `json:"Status"`
	WordsPerMinute    string   `json:"WordsPerMinute"`
	SampleRateHertz   string   `json:"SampleRateHertz"`
	StyleList         []string `json:"StyleList,omitempty"`
}

// AzureClient wraps the Azure Speech REST API
type AzureClient struct {
	subscriptionKey string
	region          string
	rateLimiter     *rate.Limiter
	httpClient      *http.Client
	customVoices    map[string]string // Custom voice mappings (overrides)
	voiceCache      map[string]string // Cached locale -> voice mappings from Azure
	voiceCacheMu    sync.RWMutex      // Protects voiceCache
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
		voiceCache:      make(map[string]string),
	}
}

// FetchVoiceList fetches available voices from Azure and populates the voice cache
func (a *AzureClient) FetchVoiceList() error {
	ctx := context.Background()
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/voices/list", a.region)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Ocp-Apim-Subscription-Key", a.subscriptionKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Azure API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var voices []Voice
	if err := json.NewDecoder(resp.Body).Decode(&voices); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Build voice cache: prefer Neural voices, prefer female voices as default
	a.voiceCacheMu.Lock()
	defer a.voiceCacheMu.Unlock()

	for _, voice := range voices {
		// Only use Neural voices
		if voice.VoiceType != "Neural" {
			continue
		}

		locale := voice.Locale

		// If this locale doesn't have a voice yet, use this one
		if _, exists := a.voiceCache[locale]; !exists {
			a.voiceCache[locale] = voice.ShortName
			continue
		}

		// If we already have a voice but this one is female and the existing is male, prefer female
		if voice.Gender == "Female" {
			a.voiceCache[locale] = voice.ShortName
		}
	}

	log.Printf("Loaded %d neural voices from Azure covering %d locales", len(voices), len(a.voiceCache))
	return nil
}

// SynthesizeToMP3 synthesizes text to speech and returns MP3 audio data
func (a *AzureClient) SynthesizeToMP3(text, languageCode string) ([]byte, error) {
	// Wait for rate limiter before making API call
	ctx := context.Background()
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	// Get voice name for language
	voiceName, err := a.getVoiceNameForLanguage(languageCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get voice for language %s: %w", languageCode, err)
	}

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
// Priority order:
// 1. Custom voice exact match (e.g., es-MX in config)
// 2. Azure cache exact match (e.g., es-MX from Azure)
// 3. Custom voice base language (e.g., es in config as fallback)
// 4. Azure cache base language (e.g., es from Azure as fallback)
func (a *AzureClient) getVoiceNameForLanguage(languageCode string) (string, error) {
	// 1. Check custom voice mapping for exact match
	if a.customVoices != nil {
		if voice, ok := a.customVoices[languageCode]; ok {
			return voice, nil
		}
	}

	// 2. Check dynamic voice cache from Azure for exact match
	a.voiceCacheMu.RLock()
	if voice, ok := a.voiceCache[languageCode]; ok {
		a.voiceCacheMu.RUnlock()
		return voice, nil
	}
	a.voiceCacheMu.RUnlock()

	// Extract base language for fallback checks
	var baseLanguage string
	if len(languageCode) > 2 && languageCode[2] == '-' {
		baseLanguage = languageCode[:2]

		// 3. Check custom voice mapping for base language
		if a.customVoices != nil {
			if voice, ok := a.customVoices[baseLanguage]; ok {
				return voice, nil
			}
		}

		// 4. Check Azure cache for base language
		a.voiceCacheMu.RLock()
		if voice, ok := a.voiceCache[baseLanguage]; ok {
			a.voiceCacheMu.RUnlock()
			return voice, nil
		}
		a.voiceCacheMu.RUnlock()
	}

	// If voice cache is empty, it means FetchVoiceList hasn't been called
	a.voiceCacheMu.RLock()
	cacheSize := len(a.voiceCache)
	a.voiceCacheMu.RUnlock()

	if cacheSize == 0 {
		return "", fmt.Errorf("voice cache not initialized - call FetchVoiceList first")
	}

	// No matching voice found
	return "", fmt.Errorf("no voice available for language code: %s", languageCode)
}
