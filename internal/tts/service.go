package tts

import (
	"fmt"
	"log"
)

// Service provides TTS functionality with caching
type Service struct {
	cache       *Cache
	azureClient *AzureClient
}

// NewService creates a new TTS service
func NewService(cache *Cache, azureClient *AzureClient) *Service {
	return &Service{
		cache:       cache,
		azureClient: azureClient,
	}
}

// GetAudio retrieves audio for the given text and language
// It first checks the cache (unless force is true), and if not found, fetches from Azure
func (s *Service) GetAudio(text, languageCode string, forceRefresh bool) (audioData []byte, cacheKey string, cached bool, err error) {
	// Try to get from cache first (unless force refresh is requested)
	if !forceRefresh {
		cachedAudio, err := s.cache.Get(text, languageCode)
		if err != nil {
			return nil, "", false, fmt.Errorf("cache lookup failed: %w", err)
		}

		if cachedAudio != nil {
			log.Printf("Cache hit for text: %s (language: %s)", text, languageCode)
			return cachedAudio.AudioData, cachedAudio.CacheKey, true, nil
		}
	} else {
		log.Printf("Force refresh requested for text: %s (language: %s)", text, languageCode)
	}

	// Cache miss - fetch from Azure
	log.Printf("Cache miss for text: %s (language: %s), fetching from Azure", text, languageCode)
	audioData, err = s.azureClient.SynthesizeToMP3(text, languageCode)
	if err != nil {
		return nil, "", false, fmt.Errorf("Azure synthesis failed: %w", err)
	}

	// Store in cache
	cacheKey, err = s.cache.Put(text, languageCode, audioData)
	if err != nil {
		// Don't fail the request if caching fails, just log the error
		log.Printf("Warning: failed to cache audio: %v", err)
		cacheKey = GenerateCacheKey(text, languageCode)
	} else {
		if forceRefresh {
			log.Printf("Force refreshed and updated cache for text: %s (cache key: %s, size: %d bytes)",
				text, cacheKey, len(audioData))
		} else {
			log.Printf("Cached audio for text: %s (cache key: %s, size: %d bytes)",
				text, cacheKey, len(audioData))
		}
	}

	return audioData, cacheKey, false, nil
}

// GetCachedAudio retrieves audio only from cache, without fetching
func (s *Service) GetCachedAudio(text, languageCode string) (audioData []byte, cacheKey string, found bool, err error) {
	cachedAudio, err := s.cache.Get(text, languageCode)
	if err != nil {
		return nil, "", false, fmt.Errorf("cache lookup failed: %w", err)
	}

	if cachedAudio == nil {
		return nil, GenerateCacheKey(text, languageCode), false, nil
	}

	return cachedAudio.AudioData, cachedAudio.CacheKey, true, nil
}

// DeleteCached removes audio from cache
func (s *Service) DeleteCached(text, languageCode string) (cacheKey string, deleted bool, err error) {
	cacheKey, deleted, err = s.cache.Delete(text, languageCode)
	if err != nil {
		return cacheKey, false, fmt.Errorf("cache delete failed: %w", err)
	}

	if deleted {
		log.Printf("Deleted cached audio for text: %s (language: %s, cache key: %s)", text, languageCode, cacheKey)
	} else {
		log.Printf("No cached audio found to delete for text: %s (language: %s, cache key: %s)", text, languageCode, cacheKey)
	}

	return cacheKey, deleted, nil
}

// GetCacheStats returns statistics about the cache
func (s *Service) GetCacheStats() (map[string]interface{}, error) {
	return s.cache.GetStats()
}

// Close closes the service and releases resources
func (s *Service) Close() error {
	return s.cache.Close()
}
