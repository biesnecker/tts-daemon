package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	pb "com.biesnecker/tts-daemon/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultAddress = "localhost:50051"
	defaultTimeout = 30 * time.Second
)

var verbose bool

func logInfo(format string, v ...interface{}) {
	if verbose {
		fmt.Printf(format, v...)
	}
}

func main() {
	// Command line flags
	address := flag.String("address", defaultAddress, "Daemon server address")
	mcpMode := flag.Bool("mcp", false, "Run in MCP mode")
	playMode := flag.Bool("play", false, "Play audio (default: just fetch)")
	language := flag.String("lang", "en-US", "Language code (e.g., en-US, fr-FR, es-ES)")
	cacheOnly := flag.Bool("cache-only", false, "Only check cache, don't fetch from Azure")
	forceRefresh := flag.Bool("force", false, "Force refresh from Azure, bypassing cache")
	flag.BoolVar(forceRefresh, "f", false, "Force refresh from Azure, bypassing cache (shorthand)")
	deleteMode := flag.Bool("D", false, "Delete cached entry")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")
	flag.BoolVar(verboseFlag, "v", false, "Enable verbose output (shorthand)")
	flag.Parse()

	verbose = *verboseFlag

	if *mcpMode {
		runMCPServer(*address)
	} else {
		runCLI(*address, *playMode, *language, *cacheOnly, *forceRefresh, *deleteMode, flag.Args())
	}
}

func runCLI(address string, playMode bool, language string, cacheOnly bool, forceRefresh bool, deleteMode bool, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: client [options] <text>\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	text := args[0]

	// Connect to daemon
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to daemon at %s: %v", address, err)
	}
	defer conn.Close()

	client := pb.NewTTSServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	req := &pb.TTSRequest{
		Text:         text,
		LanguageCode: language,
		ForceRefresh: forceRefresh,
	}

	if deleteMode {
		// Delete cached entry
		resp, err := client.DeleteCached(ctx, req)
		if err != nil {
			log.Fatalf("DeleteCached failed: %v", err)
		}

		if !resp.Success {
			fmt.Fprintf(os.Stderr, "Failed to delete: %s\n", resp.Message)
			logInfo("Cache key: %s\n", resp.CacheKey)
			os.Exit(1)
		}

		logInfo("%s\n", resp.Message)
		logInfo("Cache key: %s\n", resp.CacheKey)
	} else if cacheOnly {
		// Get cached audio only
		resp, err := client.GetCachedAudio(ctx, req)
		if err != nil {
			log.Fatalf("GetCachedAudio failed: %v", err)
		}

		if !resp.Cached {
			fmt.Fprintln(os.Stderr, "Audio not found in cache")
			logInfo("Cache key: %s\n", resp.CacheKey)
			os.Exit(1)
		}

		logInfo("Audio found in cache\n")
		logInfo("Cache key: %s\n", resp.CacheKey)
		logInfo("Audio size: %d bytes\n", resp.AudioSize)
	} else if playMode {
		// Play audio
		resp, err := client.PlayTTS(ctx, req)
		if err != nil {
			log.Fatalf("PlayTTS failed: %v", err)
		}

		if !resp.Success {
			log.Fatalf("Playback failed: %s", resp.Message)
		}

		logInfo("%s\n", resp.Message)
		if resp.WasCached {
			logInfo("(from cache)\n")
		} else {
			logInfo("(fetched from Azure)\n")
		}
	} else {
		// Just fetch audio
		resp, err := client.FetchTTS(ctx, req)
		if err != nil {
			log.Fatalf("FetchTTS failed: %v", err)
		}

		logInfo("Audio fetched successfully\n")
		logInfo("Cache key: %s\n", resp.CacheKey)
		logInfo("Audio size: %d bytes\n", resp.AudioSize)
		if resp.Cached {
			logInfo("(from cache)\n")
		} else {
			logInfo("(fetched from Azure)\n")
		}
	}
}

// MCP (Model Context Protocol) implementation
type MCPServer struct {
	address string
}

type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      interface{}            `json:"id"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func runMCPServer(address string) {
	server := &MCPServer{address: address}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	// MCP logs go to stderr to not interfere with JSON RPC on stdout
	mcpLog := log.New(os.Stderr, "", log.LstdFlags)
	if verbose {
		mcpLog.Println("MCP server started, reading from stdin...")
	}

	for {
		var req MCPRequest
		if err := decoder.Decode(&req); err != nil {
			if verbose {
				mcpLog.Printf("Error decoding request: %v", err)
			}
			break
		}

		if verbose {
			mcpLog.Printf("Received MCP request: %s", req.Method)
		}

		var resp MCPResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "tts-daemon",
					"version": "1.0.0",
				},
			}

		case "tools/list":
			resp.Result = map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "fetch_tts",
						"description": "Fetch and cache text-to-speech audio for the given text",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"text": map[string]interface{}{
									"type":        "string",
									"description": "The text to convert to speech",
								},
								"language_code": map[string]interface{}{
									"type":        "string",
									"description": "Language code (e.g., en-US, fr-FR, es-ES)",
									"default":     "en-US",
								},
							},
							"required": []string{"text"},
						},
					},
					{
						"name":        "play_tts",
						"description": "Fetch (if needed), cache, and play text-to-speech audio",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"text": map[string]interface{}{
									"type":        "string",
									"description": "The text to convert to speech and play",
								},
								"language_code": map[string]interface{}{
									"type":        "string",
									"description": "Language code (e.g., en-US, fr-FR, es-ES)",
									"default":     "en-US",
								},
							},
							"required": []string{"text"},
						},
					},
				},
			}

		case "tools/call":
			result, err := server.handleToolCall(req.Params)
			if err != nil {
				resp.Error = &MCPError{
					Code:    -32603,
					Message: err.Error(),
				}
			} else {
				resp.Result = result
			}

		default:
			resp.Error = &MCPError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			}
		}

		if err := encoder.Encode(resp); err != nil {
			if verbose {
				mcpLog.Printf("Error encoding response: %v", err)
			}
			break
		}
	}
}

func (s *MCPServer) handleToolCall(params map[string]interface{}) (interface{}, error) {
	toolName, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid tool name")
	}

	arguments, ok := params["arguments"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid arguments")
	}

	// Connect to daemon
	conn, err := grpc.NewClient(s.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	client := pb.NewTTSServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Extract common parameters
	text, ok := arguments["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("missing or invalid 'text' parameter")
	}

	languageCode := "en-US"
	if lang, ok := arguments["language_code"].(string); ok && lang != "" {
		languageCode = lang
	}

	req := &pb.TTSRequest{
		Text:         text,
		LanguageCode: languageCode,
	}

	switch toolName {
	case "fetch_tts":
		resp, err := client.FetchTTS(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("FetchTTS failed: %w", err)
		}

		status := "fetched from Azure"
		if resp.Cached {
			status = "retrieved from cache"
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Audio %s successfully.\nCache key: %s\nSize: %d bytes",
						status, resp.CacheKey, resp.AudioSize),
				},
			},
		}, nil

	case "play_tts":
		resp, err := client.PlayTTS(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("PlayTTS failed: %w", err)
		}

		if !resp.Success {
			return nil, fmt.Errorf("playback failed: %s", resp.Message)
		}

		status := "fetched from Azure and played"
		if resp.WasCached {
			status = "played from cache"
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Audio %s successfully.", status),
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}
