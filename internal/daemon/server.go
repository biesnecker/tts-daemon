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
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio (from cache or fetch from Azure)
	audioData, cacheKey, cached, err := s.ttsService.GetAudio(req.Text, req.LanguageCode, req.ForceRefresh)
	if err != nil {
		return nil, fmt.Errorf("failed to get audio: %w", err)
	}

	source := "azure"
	if cached {
		source = "cache"
	}
	log.Printf("FetchTTS: lang=%s, source=%s, size=%d", req.LanguageCode, source, len(audioData))

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
	log.Printf("Warning: PlayTTS is deprecated, client should use FetchTTS")

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio (from cache or fetch from Azure) but don't play it
	_, _, cached, err := s.ttsService.GetAudio(req.Text, req.LanguageCode, req.ForceRefresh)
	if err != nil {
		return &pb.PlayResponse{
			Success:   false,
			Message:   fmt.Sprintf("failed to get audio: %v", err),
			WasCached: false,
		}, nil
	}

	return &pb.PlayResponse{
		Success:   true,
		Message:   "Audio fetched successfully (playback handled by client)",
		WasCached: cached,
	}, nil
}

// GetCachedAudio implements the GetCachedAudio RPC method
func (s *Server) GetCachedAudio(ctx context.Context, req *pb.TTSRequest) (*pb.TTSResponse, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Get audio from cache only
	audioData, cacheKey, found, err := s.ttsService.GetCachedAudio(req.Text, req.LanguageCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached audio: %w", err)
	}

	if !found {
		return &pb.TTSResponse{
			Cached:    false,
			AudioData: nil,
			CacheKey:  cacheKey,
			AudioSize: 0,
		}, nil
	}

	return &pb.TTSResponse{
		Cached:    true,
		AudioData: audioData,
		CacheKey:  cacheKey,
		AudioSize: int64(len(audioData)),
	}, nil
}

// DeleteCached implements the DeleteCached RPC method
func (s *Server) DeleteCached(ctx context.Context, req *pb.TTSRequest) (*pb.DeleteResponse, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.LanguageCode == "" {
		return nil, fmt.Errorf("language_code is required")
	}

	// Delete from cache
	cacheKey, deleted, err := s.ttsService.DeleteCached(req.Text, req.LanguageCode)
	if err != nil {
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

	log.Printf("DeleteCached: lang=%s, key=%s", req.LanguageCode, cacheKey[:12])
	return &pb.DeleteResponse{
		Success:  true,
		Message:  "Cache entry deleted successfully",
		CacheKey: cacheKey,
	}, nil
}
