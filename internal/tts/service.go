package tts

import (
	"fmt"
	"log"
	"sync"
)

// inFlightFetch tracks an ongoing fetch operation
type inFlightFetch struct {
	done      chan struct{}
	audioData []byte
	cacheKey  string
	cached    bool
	err       error
}

// Service provides TTS functionality with caching
type Service struct {
	cache       *Cache
	azureClient *AzureClient

	// In-flight fetch tracking to deduplicate concurrent requests
	inFlightMu sync.Mutex
	inFlight   map[string]*inFlightFetch
}

// NewService creates a new TTS service
func NewService(cache *Cache, azureClient *AzureClient) *Service {
	return &Service{
		cache:       cache,
		azureClient: azureClient,
		inFlight:    make(map[string]*inFlightFetch),
	}
}

// GetAudio retrieves audio for the given text and language
// It first checks the cache (unless force is true), and if not found, fetches from Azure
// Concurrent requests for the same text/language will wait on the same fetch operation
func (s *Service) GetAudio(text, languageCode string, forceRefresh bool) (audioData []byte, cacheKey string, cached bool, err error) {
	// Try to get from cache first (unless force refresh is requested)
	if !forceRefresh {
		cachedAudio, err := s.cache.Get(text, languageCode)
		if err != nil {
			return nil, "", false, fmt.Errorf("cache lookup failed: %w", err)
		}

		if cachedAudio != nil {
			return cachedAudio.AudioData, cachedAudio.CacheKey, true, nil
		}
	}

	// Cache miss - check if there's already an in-flight fetch for this item
	key := GenerateCacheKey(text, languageCode)

	// Check for existing in-flight fetch
	s.inFlightMu.Lock()
	if flight, exists := s.inFlight[key]; exists {
		// Another goroutine is already fetching this, wait for it
		s.inFlightMu.Unlock()
		<-flight.done
		return flight.audioData, flight.cacheKey, flight.cached, flight.err
	}

	// No in-flight fetch, create one
	flight := &inFlightFetch{
		done: make(chan struct{}),
	}
	s.inFlight[key] = flight
	s.inFlightMu.Unlock()

	// Perform the fetch (outside the lock)
	audioData, err = s.azureClient.SynthesizeToMP3(text, languageCode)
	if err != nil {
		flight.err = fmt.Errorf("Azure synthesis failed: %w", err)
	} else {
		// Store in cache
		cacheKey, err = s.cache.Put(text, languageCode, audioData)
		if err != nil {
			// Don't fail the request if caching fails, just log the error
			log.Printf("Warning: caching failed: %v", err)
			cacheKey = key
		}

		flight.audioData = audioData
		flight.cacheKey = cacheKey
		flight.cached = false
	}

	// Remove from in-flight map and signal completion
	s.inFlightMu.Lock()
	delete(s.inFlight, key)
	s.inFlightMu.Unlock()
	close(flight.done)

	return flight.audioData, flight.cacheKey, flight.cached, flight.err
}

// BulkGetAudio retrieves audio for multiple text/language pairs concurrently
// Returns a slice of results in the same order as the requests
func (s *Service) BulkGetAudio(requests []struct{ Text, LanguageCode string }, forceRefresh bool) []struct {
	AudioData []byte
	CacheKey  string
	Cached    bool
	Err       error
} {
	results := make([]struct {
		AudioData []byte
		CacheKey  string
		Cached    bool
		Err       error
	}, len(requests))

	// Use a WaitGroup to fetch all items concurrently
	var wg sync.WaitGroup
	for i, req := range requests {
		wg.Add(1)
		go func(idx int, text, lang string) {
			defer wg.Done()
			audioData, cacheKey, cached, err := s.GetAudio(text, lang, forceRefresh)
			results[idx].AudioData = audioData
			results[idx].CacheKey = cacheKey
			results[idx].Cached = cached
			results[idx].Err = err
		}(i, req.Text, req.LanguageCode)
	}
	wg.Wait()

	return results
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
