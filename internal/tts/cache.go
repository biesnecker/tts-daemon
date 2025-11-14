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
)

// Cache manages the audio clip cache
type Cache struct {
	db *sql.DB
}

// CachedAudio represents a cached audio clip
type CachedAudio struct {
	CacheKey     string
	Text         string
	LanguageCode string
	AudioData    []byte
	CreatedAt    int64
}

// NewCache creates a new cache instance
func NewCache(dbPath string) (*Cache, error) {
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

	// Create cache instance
	cache := &Cache{db: db}

	// Initialize schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return cache, nil
}

// initSchema creates the database schema
func (c *Cache) initSchema() error {
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
		`SELECT cache_key, text, language_code, audio_data, created_at
		 FROM audio_cache WHERE cache_key = ?`,
		cacheKey,
	).Scan(
		&audio.CacheKey,
		&audio.Text,
		&audio.LanguageCode,
		&audio.AudioData,
		&audio.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	return &audio, nil
}

// Put stores audio in cache
func (c *Cache) Put(text, languageCode string, audioData []byte) (string, error) {
	cacheKey := GenerateCacheKey(text, languageCode)
	now := getCurrentTimestamp()

	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO audio_cache
		 (cache_key, text, language_code, audio_data, audio_size, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		cacheKey,
		text,
		languageCode,
		audioData,
		len(audioData),
		now,
	)

	if err != nil {
		return "", fmt.Errorf("failed to insert into cache: %w", err)
	}

	return cacheKey, nil
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

// Close closes the database connection
func (c *Cache) Close() error {
	return c.db.Close()
}

// getCurrentTimestamp returns the current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
