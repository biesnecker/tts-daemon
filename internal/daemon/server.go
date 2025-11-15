package daemon

import (
	"context"
	"fmt"
	"log"

	pb "com.biesnecker/tts-daemon/proto"
	"com.biesnecker/tts-daemon/internal/tts"
)

// Server implements the gRPC TTSService
type Server struct {
	pb.UnimplementedTTSServiceServer
	ttsService *tts.Service
}

// NewServer creates a new gRPC server
func NewServer(ttsService *tts.Service) *Server {
	return &Server{
		ttsService: ttsService,
	}
}

// FetchTTS implements the FetchTTS RPC method
func (s *Server) FetchTTS(ctx context.Context, req *pb.TTSRequest) (*pb.TTSResponse, error) {
	log.Printf("FetchTTS request: text=%q, language=%s, force=%v", req.Text, req.LanguageCode, req.ForceRefresh)

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio (from cache or fetch from Azure)
	audioData, cacheKey, cached, err := s.ttsService.GetAudio(req.Text, req.LanguageCode, req.ForceRefresh)
	if err != nil {
		log.Printf("FetchTTS error: %v", err)
		return nil, fmt.Errorf("failed to get audio: %w", err)
	}

	log.Printf("FetchTTS success: cached=%v, cache_key=%s, size=%d bytes",
		cached, cacheKey, len(audioData))

	return &pb.TTSResponse{
		Cached:    cached,
		AudioData: audioData,
		CacheKey:  cacheKey,
		AudioSize: int64(len(audioData)),
	}, nil
}

// PlayTTS implements the PlayTTS RPC method
// NOTE: This method is deprecated. Clients should use FetchTTS and play audio locally.
// Kept for backward compatibility - just returns success without playing.
func (s *Server) PlayTTS(ctx context.Context, req *pb.TTSRequest) (*pb.PlayResponse, error) {
	log.Printf("PlayTTS request (deprecated): text=%q, language=%s, force=%v", req.Text, req.LanguageCode, req.ForceRefresh)
	log.Printf("WARNING: PlayTTS is deprecated. Client should use FetchTTS and play audio locally.")

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio (from cache or fetch from Azure) but don't play it
	_, cacheKey, cached, err := s.ttsService.GetAudio(req.Text, req.LanguageCode, req.ForceRefresh)
	if err != nil {
		log.Printf("PlayTTS error getting audio: %v", err)
		return &pb.PlayResponse{
			Success:   false,
			Message:   fmt.Sprintf("failed to get audio: %v", err),
			WasCached: false,
		}, nil
	}

	log.Printf("PlayTTS success (audio fetched but not played): cached=%v, cache_key=%s", cached, cacheKey)

	return &pb.PlayResponse{
		Success:   true,
		Message:   "Audio fetched successfully (playback handled by client)",
		WasCached: cached,
	}, nil
}

// GetCachedAudio implements the GetCachedAudio RPC method
func (s *Server) GetCachedAudio(ctx context.Context, req *pb.TTSRequest) (*pb.TTSResponse, error) {
	log.Printf("GetCachedAudio request: text=%q, language=%s", req.Text, req.LanguageCode)

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio from cache only
	audioData, cacheKey, found, err := s.ttsService.GetCachedAudio(req.Text, req.LanguageCode)
	if err != nil {
		log.Printf("GetCachedAudio error: %v", err)
		return nil, fmt.Errorf("failed to get cached audio: %w", err)
	}

	if !found {
		log.Printf("GetCachedAudio: not found in cache, cache_key=%s", cacheKey)
		return &pb.TTSResponse{
			Cached:    false,
			AudioData: nil,
			CacheKey:  cacheKey,
			AudioSize: 0,
		}, nil
	}

	log.Printf("GetCachedAudio success: cache_key=%s, size=%d bytes", cacheKey, len(audioData))

	return &pb.TTSResponse{
		Cached:    true,
		AudioData: audioData,
		CacheKey:  cacheKey,
		AudioSize: int64(len(audioData)),
	}, nil
}

// DeleteCached implements the DeleteCached RPC method
func (s *Server) DeleteCached(ctx context.Context, req *pb.TTSRequest) (*pb.DeleteResponse, error) {
	log.Printf("DeleteCached request: text=%q, language=%s", req.Text, req.LanguageCode)

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Delete from cache
	cacheKey, deleted, err := s.ttsService.DeleteCached(req.Text, req.LanguageCode)
	if err != nil {
		log.Printf("DeleteCached error: %v", err)
		return &pb.DeleteResponse{
			Success:  false,
			Message:  fmt.Sprintf("Failed to delete: %v", err),
			CacheKey: cacheKey,
		}, nil
	}

	if !deleted {
		return &pb.DeleteResponse{
			Success:  false,
			Message:  "Entry not found in cache",
			CacheKey: cacheKey,
		}, nil
	}

	return &pb.DeleteResponse{
		Success:  true,
		Message:  "Cache entry deleted successfully",
		CacheKey: cacheKey,
	}, nil
}
