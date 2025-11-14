package player

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
)

// Player handles audio playback
type Player struct {
	sampleRate  beep.SampleRate
	bufferSize  int
	initialized bool
	mu          sync.Mutex
}

// NewPlayer creates a new audio player
func NewPlayer(sampleRate, bufferSize int) *Player {
	return &Player{
		sampleRate:  beep.SampleRate(sampleRate),
		bufferSize:  bufferSize,
		initialized: false,
	}
}

// PlayMP3 plays MP3 audio data
func (p *Player) PlayMP3(audioData []byte) error {
	p.mu.Lock()
	// Initialize speaker if not already done
	if !p.initialized {
		err := speaker.Init(p.sampleRate, p.sampleRate.N(time.Second/10))
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("failed to initialize speaker: %w", err)
		}
		p.initialized = true
	}
	p.mu.Unlock()

	// Create a reader from the audio data
	reader := bytes.NewReader(audioData)

	// Decode MP3
	streamer, format, err := mp3.Decode(io.NopCloser(reader))
	if err != nil {
		return fmt.Errorf("failed to decode MP3: %w", err)
	}
	defer streamer.Close()

	// Resample if the MP3 sample rate doesn't match our speaker's sample rate
	var resampled beep.Streamer = streamer
	if format.SampleRate != p.sampleRate {
		resampled = beep.Resample(4, format.SampleRate, p.sampleRate, streamer)
	}

	// Create a done channel to wait for playback to finish
	done := make(chan bool)

	// Play the audio
	speaker.Play(beep.Seq(resampled, beep.Callback(func() {
		done <- true
	})))

	// Wait for playback to complete
	<-done

	return nil
}

// Close cleans up the player resources
func (p *Player) Close() {
	if p.initialized {
		speaker.Clear()
	}
}
