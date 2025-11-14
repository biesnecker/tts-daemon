package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "com.biesnecker/tts-daemon/proto"
	"com.biesnecker/tts-daemon/internal/config"
	"com.biesnecker/tts-daemon/internal/daemon"
	"com.biesnecker/tts-daemon/internal/player"
	"com.biesnecker/tts-daemon/internal/tts"
	"google.golang.org/grpc"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file (default: ~/.tts-daemon/config.yaml)")
	flag.Parse()

	// Load configuration
	var cfg *config.Config
	var err error

	if *configPath == "" {
		defaultPath, err := config.GetDefaultConfigPath()
		if err != nil {
			log.Fatalf("Failed to get default config path: %v", err)
		}
		*configPath = defaultPath
	}

	cfg, err = config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", *configPath, err)
	}

	log.Printf("Loaded configuration from %s", *configPath)
	log.Printf("Azure region: %s", cfg.Azure.Region)
	log.Printf("Max QPS: %.2f", cfg.Azure.MaxQPS)
	log.Printf("Database path: %s", cfg.Database.Path)
	log.Printf("Database compression: %v", cfg.Database.Compression)
	if cfg.Database.MaxSizeMB > 0 {
		log.Printf("Database max size: %d MB", cfg.Database.MaxSizeMB)
	} else {
		log.Printf("Database max size: unlimited")
	}
	log.Printf("Server address: %s:%d", cfg.Server.Address, cfg.Server.Port)

	// Initialize cache
	cache, err := tts.NewCache(cfg.Database.Path, cfg.Database.Compression, cfg.Database.MaxSizeMB)
	if err != nil {
		log.Fatalf("Failed to initialize cache: %v", err)
	}
	defer cache.Close()
	log.Printf("Cache initialized at %s", cfg.Database.Path)

	// Print cache stats
	stats, err := cache.GetStats()
	if err != nil {
		log.Printf("Warning: failed to get cache stats: %v", err)
	} else {
		if cfg.Database.MaxSizeMB > 0 {
			log.Printf("Cache stats: %d clips, %.2f MB / %.2f MB (%.1f%% full)",
				stats["total_clips"], stats["size_mb"], stats["max_size_mb"], stats["usage_percent"])
		} else {
			log.Printf("Cache stats: %d clips, %.2f MB", stats["total_clips"], stats["size_mb"])
		}
	}

	// Initialize Azure TTS client with rate limiting
	azureClient := tts.NewAzureClient(cfg.Azure.SubscriptionKey, cfg.Azure.Region, cfg.Azure.MaxQPS, cfg.Azure.Voices)
	log.Printf("Azure TTS client initialized with rate limit: %.2f QPS", cfg.Azure.MaxQPS)
	if len(cfg.Azure.Voices) > 0 {
		log.Printf("Custom voice mappings configured: %d language(s)", len(cfg.Azure.Voices))
		for lang, voice := range cfg.Azure.Voices {
			log.Printf("  %s -> %s", lang, voice)
		}
	}

	// Initialize TTS service
	ttsService := tts.NewService(cache, azureClient)
	defer ttsService.Close()

	// Initialize audio player
	audioPlayer := player.NewPlayer(cfg.Audio.SampleRate, cfg.Audio.BufferSize)
	defer audioPlayer.Close()
	log.Printf("Audio player initialized (sample rate: %d Hz, buffer size: %d)",
		cfg.Audio.SampleRate, cfg.Audio.BufferSize)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	ttsServer := daemon.NewServer(ttsService, audioPlayer)
	pb.RegisterTTSServiceServer(grpcServer, ttsServer)

	// Start listening
	address := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", address, err)
	}

	log.Printf("TTS daemon listening on %s", address)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping server...")
		grpcServer.GracefulStop()
	}()

	// Start serving
	log.Println("TTS daemon started successfully")
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
