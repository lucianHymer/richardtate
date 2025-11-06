package transcription

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// TranscriptionPipeline handles the complete audio-to-text pipeline
// Flow: Raw Audio → RNNoise → VAD/Chunker → Whisper → Results
type TranscriptionPipeline struct {
	whisper     *WhisperTranscriber
	rnnoise     *RNNoiseProcessor
	chunker     *SmartChunker
	resultChan  chan TranscriptionResult
	mu          sync.RWMutex
	active      bool
	debugWAV    bool // Enable WAV file debugging
	log         *logger.ContextLogger
}

// TranscriptionResult holds transcription output
type TranscriptionResult struct {
	Text      string
	Timestamp int64 // Unix timestamp in milliseconds
	Error     error
}

// PipelineConfig holds configuration for the transcription pipeline
type PipelineConfig struct {
	WhisperConfig      WhisperConfig
	RNNoiseModelPath   string        // Path to RNNoise model
	SilenceThreshold   time.Duration // Silence duration to trigger chunk (1s default)
	MinChunkDuration   time.Duration // Minimum chunk duration
	MaxChunkDuration   time.Duration // Maximum chunk duration
	VADEnergyThreshold float64       // VAD energy threshold
	ResultChannelSize  int           // Size of result channel buffer
	EnableDebugWAV     bool          // Save WAV files for debugging
}

// NewTranscriptionPipeline creates a new transcription pipeline
func NewTranscriptionPipeline(config PipelineConfig) (*TranscriptionPipeline, error) {
	// Create logger
	log := config.WhisperConfig.Logger.With("pipeline")

	// Create Whisper transcriber
	whisper, err := NewWhisperTranscriber(config.WhisperConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper transcriber: %w", err)
	}

	// Create RNNoise processor
	rnnoise, err := NewRNNoiseProcessor(config.RNNoiseModelPath, config.WhisperConfig.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create RNNoise processor: %w", err)
	}

	// Result channel
	resultChanSize := config.ResultChannelSize
	if resultChanSize == 0 {
		resultChanSize = 10
	}
	resultChan := make(chan TranscriptionResult, resultChanSize)

	pipeline := &TranscriptionPipeline{
		whisper:    whisper,
		rnnoise:    rnnoise,
		resultChan: resultChan,
		active:     false,
		debugWAV:   config.EnableDebugWAV,
		log:        log,
	}

	// Create smart chunker with VAD
	pipeline.chunker = NewSmartChunker(SmartChunkerConfig{
		SampleRate:         16000,
		SilenceThreshold:   config.SilenceThreshold,
		MinChunkDuration:   config.MinChunkDuration,
		MaxChunkDuration:   config.MaxChunkDuration,
		VADEnergyThreshold: config.VADEnergyThreshold,
		ChunkReadyCallback: pipeline.transcribeChunk,
		Logger:             config.WhisperConfig.Logger,
	})

	return pipeline, nil
}

// ProcessChunk processes an incoming audio chunk through the pipeline
// Flow: Raw PCM → RNNoise → Chunker (with VAD) → [triggers transcription on silence]
func (p *TranscriptionPipeline) ProcessChunk(audioData []byte, timestamp int64) error {
	p.mu.RLock()
	if !p.active {
		p.mu.RUnlock()
		return fmt.Errorf("pipeline not active")
	}
	p.mu.RUnlock()

	// Step 1: Denoise with RNNoise
	denoisedBytes, err := p.rnnoise.ProcessBytes(audioData)
	if err != nil {
		p.log.Warn("RNNoise error: %v", err)
		// Continue with original audio on error
		denoisedBytes = audioData
	}

	// Step 2: Convert to int16 samples for chunker
	samples := make([]int16, len(denoisedBytes)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(denoisedBytes[i*2]) | int16(denoisedBytes[i*2+1])<<8
	}

	// Step 3: Process through smart chunker (includes VAD)
	// The chunker will call transcribeChunk() when a chunk is ready
	p.chunker.ProcessSamples(samples)

	return nil
}

// transcribeChunk is called by the chunker when a chunk is ready for transcription
func (p *TranscriptionPipeline) transcribeChunk(samples []int16) {
	duration := float64(len(samples)) / 16000.0

	// Save debug WAV if enabled
	if p.debugWAV {
		p.saveDebugWAV(samples)
	}

	// Convert int16 samples to float32 for Whisper
	floatSamples := make([]float32, len(samples))
	for i, sample := range samples {
		floatSamples[i] = float32(sample) / 32768.0
	}

	// Transcribe
	text, err := p.whisper.Transcribe(floatSamples)

	// Send result
	result := TranscriptionResult{
		Text:      text,
		Timestamp: currentTimeMillis(),
		Error:     err,
	}

	select {
	case p.resultChan <- result:
		if err != nil {
			p.log.ErrorWithFields("Transcription failed", map[string]interface{}{
				"duration": fmt.Sprintf("%.1fs", duration),
				"error":    err.Error(),
			})
		} else {
			p.log.InfoWithFields("Transcription complete", map[string]interface{}{
				"duration": fmt.Sprintf("%.1fs", duration),
				"text":     text,
			})
		}
	default:
		p.log.WarnWithFields("Result dropped (channel full)", map[string]interface{}{
			"duration": fmt.Sprintf("%.1fs", duration),
		})
	}
}

// saveDebugWAV saves a chunk to WAV file for debugging
func (p *TranscriptionPipeline) saveDebugWAV(samples []int16) {
	// Convert samples to bytes
	pcmData := make([]byte, len(samples)*2)
	for i, sample := range samples {
		pcmData[i*2] = byte(sample)
		pcmData[i*2+1] = byte(sample >> 8)
	}

	wavPath := fmt.Sprintf("/tmp/chunk-%d.wav", time.Now().Unix())
	if err := saveWAV(wavPath, pcmData, 16000, 1, 16); err != nil {
		p.log.Warn("Failed to save debug WAV: %v", err)
	} else {
		p.log.Debug("Saved chunk to %s", wavPath)
	}
}

// Start activates the pipeline
func (p *TranscriptionPipeline) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return fmt.Errorf("pipeline already active")
	}

	p.active = true

	// Reset components
	p.chunker.Reset()
	p.rnnoise.Reset()

	return nil
}

// Stop deactivates the pipeline and flushes any remaining audio
func (p *TranscriptionPipeline) Stop() error {
	p.mu.Lock()
	if !p.active {
		p.mu.Unlock()
		return fmt.Errorf("pipeline not active")
	}
	p.active = false
	p.mu.Unlock()

	// Flush any remaining audio in chunker
	p.chunker.Flush()

	// Flush any remaining audio in RNNoise buffer
	remainingSamples := p.rnnoise.Flush()
	if len(remainingSamples) > 0 {
		p.chunker.ProcessSamples(remainingSamples)
		p.chunker.Flush() // Flush again after adding RNNoise remainder
	}

	return nil
}

// Results returns the channel for receiving transcription results
func (p *TranscriptionPipeline) Results() <-chan TranscriptionResult {
	return p.resultChan
}

// Close releases all resources
func (p *TranscriptionPipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.active = false

	if p.whisper != nil {
		p.whisper.Close()
	}

	if p.rnnoise != nil {
		p.rnnoise.Close()
	}

	close(p.resultChan)

	return nil
}

// IsActive returns whether the pipeline is currently active
func (p *TranscriptionPipeline) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// GetStats returns current pipeline statistics
func (p *TranscriptionPipeline) GetStats() PipelineStats {
	chunkerStats := p.chunker.GetStats()

	return PipelineStats{
		Active:         p.IsActive(),
		ChunkerStats:   chunkerStats,
	}
}

// PipelineStats holds pipeline statistics
type PipelineStats struct {
	Active       bool
	ChunkerStats ChunkerStats
}

// saveWAV writes PCM audio data to a WAV file
func saveWAV(filename string, pcmData []byte, sampleRate, channels, bitsPerSample int) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	dataSize := uint32(len(pcmData))
	fileSize := 36 + dataSize

	// Write WAV header
	// "RIFF" chunk
	file.WriteString("RIFF")
	binary.Write(file, binary.LittleEndian, fileSize)
	file.WriteString("WAVE")

	// "fmt " subchunk
	file.WriteString("fmt ")
	binary.Write(file, binary.LittleEndian, uint32(16))                              // Subchunk size
	binary.Write(file, binary.LittleEndian, uint16(1))                               // Audio format (1 = PCM)
	binary.Write(file, binary.LittleEndian, uint16(channels))                        // Number of channels
	binary.Write(file, binary.LittleEndian, uint32(sampleRate))                      // Sample rate
	binary.Write(file, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8)) // Byte rate
	binary.Write(file, binary.LittleEndian, uint16(channels*bitsPerSample/8))        // Block align
	binary.Write(file, binary.LittleEndian, uint16(bitsPerSample))                   // Bits per sample

	// "data" subchunk
	file.WriteString("data")
	binary.Write(file, binary.LittleEndian, dataSize)
	file.Write(pcmData)

	return nil
}

// Helper functions

func currentTimeMillis() int64 {
	return int64(time.Now().UnixNano() / 1000000)
}
