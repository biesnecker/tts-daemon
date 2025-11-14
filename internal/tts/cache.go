package tts

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "github.com/mattn/go-sqlite3"
	"github.com/klauspost/compress/zstd"
)

// Cache manages the audio clip cache
type Cache struct {
	db                *sql.DB
	compressionEnabled bool
	encoder           *zstd.Encoder
	decoder           *zstd.Decoder
}

// CachedAudio represents a cached audio clip
type CachedAudio struct {
	CacheKey     string
	Text         string
	LanguageCode string
	AudioData    []byte
	Compression  sql.NullString // "zstd" or NULL for uncompressed
	CreatedAt    int64
}

// NewCache creates a new cache instance
func NewCache(dbPath string, compressionEnabled bool) (*Cache, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize encoder/decoder if compression is enabled
	var encoder *zstd.Encoder
	var decoder *zstd.Decoder
	if compressionEnabled {
		// Create encoder with default compression level
		encoder, err = zstd.NewWriter(nil)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
		}

		// Create decoder
		decoder, err = zstd.NewReader(nil)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
	}

	// Create cache instance
	cache := &Cache{
		db:                db,
		compressionEnabled: compressionEnabled,
		encoder:           encoder,
		decoder:           decoder,
	}

	// Initialize schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return cache, nil
}

// initSchema creates the database schema
func (c *Cache) initSchema() error {
	// Create table if it doesn't exist
	schema := `
	CREATE TABLE IF NOT EXISTS audio_cache (
		cache_key TEXT PRIMARY KEY,
		text TEXT NOT NULL,
		language_code TEXT NOT NULL,
		audio_data BLOB NOT NULL,
		audio_size INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_language_code ON audio_cache(language_code);
	CREATE INDEX IF NOT EXISTS idx_created_at ON audio_cache(created_at);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Check if compression column exists and add it if it doesn't
	var columnExists bool
	row := c.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('audio_cache') WHERE name='compression'`)
	if err := row.Scan(&columnExists); err != nil {
		return fmt.Errorf("failed to check for compression column: %w", err)
	}

	if !columnExists {
		// Add compression column if it doesn't exist
		_, err := c.db.Exec(`ALTER TABLE audio_cache ADD COLUMN compression TEXT`)
		if err != nil {
			return fmt.Errorf("failed to add compression column: %w", err)
		}
	}

	// Create compression index if it doesn't exist
	_, err = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_compression ON audio_cache(compression)`)
	if err != nil {
		return fmt.Errorf("failed to create compression index: %w", err)
	}

	return nil
}

// NormalizeText normalizes text for consistent caching
func NormalizeText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Remove extra whitespace
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")

	// Remove punctuation from the end (but keep internal punctuation)
	text = strings.TrimRightFunc(text, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})

	return text
}

// GenerateCacheKey generates a cache key for the given text and language
func GenerateCacheKey(text, languageCode string) string {
	normalized := NormalizeText(text)
	// Include language code in hash to differentiate same text in different languages
	combined := fmt.Sprintf("%s:%s", languageCode, normalized)

	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// Get retrieves audio from cache
func (c *Cache) Get(text, languageCode string) (*CachedAudio, error) {
	cacheKey := GenerateCacheKey(text, languageCode)

	var audio CachedAudio
	err := c.db.QueryRow(
		`SELECT cache_key, text, language_code, audio_data, compression, created_at
		 FROM audio_cache WHERE cache_key = ?`,
		cacheKey,
	).Scan(
		&audio.CacheKey,
		&audio.Text,
		&audio.LanguageCode,
		&audio.AudioData,
		&audio.Compression,
		&audio.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Decompress if needed
	if audio.Compression.Valid && audio.Compression.String == "zstd" {
		if c.decoder == nil {
			return nil, fmt.Errorf("zstd decoder not initialized")
		}
		decompressed, err := c.decoder.DecodeAll(audio.AudioData, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress audio data: %w", err)
		}
		audio.AudioData = decompressed
	}

	// If compression is enabled but data is uncompressed, spawn background job to compress it
	if c.compressionEnabled && !audio.Compression.Valid {
		go c.recompressEntry(cacheKey, audio.AudioData)
	}

	return &audio, nil
}

// Put stores audio in cache
func (c *Cache) Put(text, languageCode string, audioData []byte) (string, error) {
	cacheKey := GenerateCacheKey(text, languageCode)
	now := getCurrentTimestamp()

	var dataToStore []byte
	var compression sql.NullString

	// Compress if enabled
	if c.compressionEnabled {
		if c.encoder == nil {
			return "", fmt.Errorf("zstd encoder not initialized")
		}
		compressed := c.encoder.EncodeAll(audioData, nil)
		dataToStore = compressed
		compression = sql.NullString{String: "zstd", Valid: true}
	} else {
		dataToStore = audioData
		compression = sql.NullString{Valid: false}
	}

	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO audio_cache
		 (cache_key, text, language_code, audio_data, audio_size, compression, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cacheKey,
		text,
		languageCode,
		dataToStore,
		len(dataToStore),
		compression,
		now,
	)

	if err != nil {
		return "", fmt.Errorf("failed to insert into cache: %w", err)
	}

	return cacheKey, nil
}

// recompressEntry compresses an uncompressed cache entry in the background
func (c *Cache) recompressEntry(cacheKey string, uncompressedData []byte) {
	if c.encoder == nil {
		return
	}

	// Compress the data
	compressed := c.encoder.EncodeAll(uncompressedData, nil)

	// Update the database entry
	_, err := c.db.Exec(
		`UPDATE audio_cache
		 SET audio_data = ?, audio_size = ?, compression = ?
		 WHERE cache_key = ? AND compression IS NULL`,
		compressed,
		len(compressed),
		"zstd",
		cacheKey,
	)

	if err != nil {
		// Silently fail - this is a background optimization
		// We don't want to disrupt the user experience
		return
	}
}

// Delete removes audio from cache
func (c *Cache) Delete(text, languageCode string) (string, bool, error) {
	cacheKey := GenerateCacheKey(text, languageCode)

	result, err := c.db.Exec(
		`DELETE FROM audio_cache WHERE cache_key = ?`,
		cacheKey,
	)

	if err != nil {
		return cacheKey, false, fmt.Errorf("failed to delete from cache: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return cacheKey, false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return cacheKey, rowsAffected > 0, nil
}

// GetStats returns cache statistics
func (c *Cache) GetStats() (map[string]interface{}, error) {
	var count int64
	var totalSize int64

	err := c.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(audio_size), 0) FROM audio_cache`,
	).Scan(&count, &totalSize)

	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	return map[string]interface{}{
		"total_clips": count,
		"total_size":  totalSize,
		"size_mb":     float64(totalSize) / (1024 * 1024),
	}, nil
}

// Close closes the database connection and cleanup resources
func (c *Cache) Close() error {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
	return c.db.Close()
}

// getCurrentTimestamp returns the current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
