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
// Flow: Raw Audio → RNNoise → VAD/Chunker → ASR (Whisper/Parakeet) → Results
type TranscriptionPipeline struct {
	asr          ASRTranscriber // ASR engine (Whisper or Parakeet)
	engine       string         // Engine type: "whisper" or "parakeet"
	rnnoise      *RNNoiseProcessor
	chunker      *SmartChunker
	vad          *VoiceActivityDetector // VAD for Parakeet gating (Whisper uses chunker's VAD)
	resultChan   chan TranscriptionResult
	stateTracker *ProcessingStateTracker // Tracks processing state for loading indicator
	mu           sync.RWMutex
	active       bool
	debugWAV     bool // Enable WAV file debugging
	log          *logger.ContextLogger

	// Parakeet silence accumulation state
	parakeetWasSpeaking bool      // Was the previous chunk speech?
	parakeetSilenceBuf  []float32 // Accumulated silence samples
	parakeetSilenceSent bool      // Have we sent the silence flush after speech?
}

// TranscriptionResult holds transcription output or state changes
type TranscriptionResult struct {
	Text      string
	Timestamp int64 // Unix timestamp in milliseconds
	Error     error

	// Optional: State change notification (if IsStateChange is true, Text/Error are ignored)
	IsStateChange bool
	IsProcessing  bool // Processing state (only valid if IsStateChange is true)
}

// PipelineConfig holds configuration for the transcription pipeline
type PipelineConfig struct {
	Engine                 string                 // ASR engine: "whisper" or "parakeet" (default: "whisper")
	SharedWhisperModel     *SharedWhisperModel    // Shared model across all pipelines (for Whisper engine)
	WhisperConfig          WhisperConfig          // Whisper-specific configuration
	SharedParakeetWorker   *SharedParakeetWorker  // Shared worker across all pipelines (for Parakeet engine)
	RNNoiseModelPath       string                 // Path to RNNoise model
	SilenceThreshold       time.Duration          // Silence duration to trigger chunk (1s default)
	MinChunkDuration       time.Duration          // Minimum chunk duration
	MaxChunkDuration       time.Duration          // Maximum chunk duration
	VADEnergyThreshold     float64                // VAD energy threshold
	SpeechDensityThreshold float64                // Speech density threshold for short utterances
	ResultChannelSize      int                    // Size of result channel buffer
	EnableDebugWAV         bool                   // Save WAV files for debugging
}

// NewTranscriptionPipeline creates a new transcription pipeline
func NewTranscriptionPipeline(config PipelineConfig) (*TranscriptionPipeline, error) {
	// Create logger
	log := config.WhisperConfig.Logger.With("pipeline")

	// Create ASR transcriber using factory
	asr, err := NewASRTranscriber(ASRConfig{
		Engine:               config.Engine,
		SharedWhisperModel:   config.SharedWhisperModel,
		WhisperConfig:        config.WhisperConfig,
		SharedParakeetWorker: config.SharedParakeetWorker,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ASR transcriber: %w", err)
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

	// Default engine to whisper if not specified
	engine := config.Engine
	if engine == "" {
		engine = "whisper"
	}

	pipeline := &TranscriptionPipeline{
		asr:        asr,
		engine:     engine,
		rnnoise:    rnnoise,
		resultChan: resultChan,
		active:     false,
		debugWAV:   config.EnableDebugWAV,
		log:        log,
	}

	// Create VAD for Parakeet gating (Whisper uses chunker's internal VAD)
	if engine == "parakeet" {
		pipeline.vad = NewVAD(VADConfig{
			EnergyThreshold:    config.VADEnergyThreshold,
			SilenceThresholdMs: int(config.SilenceThreshold.Milliseconds()),
			SampleRate:         16000,
		})
		log.Info("VAD initialized for Parakeet gating with threshold %.1f", config.VADEnergyThreshold)
	}

	// Create smart chunker with VAD (used by Whisper, also available for stats)
	pipeline.chunker = NewSmartChunker(SmartChunkerConfig{
		SampleRate:             16000,
		SilenceThreshold:       config.SilenceThreshold,
		MinChunkDuration:       config.MinChunkDuration,
		MaxChunkDuration:       config.MaxChunkDuration,
		VADEnergyThreshold:     config.VADEnergyThreshold,
		SpeechDensityThreshold: config.SpeechDensityThreshold,
		ChunkReadyCallback:     pipeline.transcribeChunk,
		Logger:                 config.WhisperConfig.Logger,
	})

	// Create processing state tracker (2 second idle timeout)
	pipeline.stateTracker = NewProcessingStateTracker(
		2*time.Second,
		func(isProcessing bool) {
			// Send state change through result channel
			stateResult := TranscriptionResult{
				IsStateChange: true,
				IsProcessing:  isProcessing,
				Timestamp:     currentTimeMillis(),
			}
			select {
			case pipeline.resultChan <- stateResult:
				// State change sent
			default:
				log.Warn("State change dropped (channel full)")
			}
		},
		config.WhisperConfig.Logger,
	)

	return pipeline, nil
}

// ProcessChunk processes an incoming audio chunk through the pipeline
// Flow for Whisper: Raw PCM → RNNoise → Chunker (with VAD) → [triggers transcription on silence]
// Flow for Parakeet: Raw PCM → RNNoise → Direct to ASR (Parakeet handles buffering internally)
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

	// Step 2: Convert to int16 samples
	samples := make([]int16, len(denoisedBytes)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(denoisedBytes[i*2]) | int16(denoisedBytes[i*2+1])<<8
	}

	// Step 3: Route based on engine type
	if p.engine == "parakeet" {
		// For Parakeet streaming: use VAD to manage silence accumulation
		// - Speech chunks: send immediately (with any accumulated silence)
		// - First silence after speech: send it (pushes Parakeet buffer)
		// - Continuing silence: accumulate, don't send (avoids refreshes)

		// Process samples through VAD to detect speech
		frameSize := 160 // 10ms at 16kHz
		hasSpeech := false
		for offset := 0; offset+frameSize <= len(samples); offset += frameSize {
			frame := samples[offset : offset+frameSize]
			if p.vad.ProcessFrame(frame) {
				hasSpeech = true
			}
		}

		// Notify state tracker of VAD state
		if hasSpeech {
			p.stateTracker.NotifySpeechDetected()
		} else {
			p.stateTracker.NotifySilence()
		}

		// Convert int16 samples to float32 for ASR
		floatSamples := make([]float32, len(samples))
		for i, sample := range samples {
			floatSamples[i] = float32(sample) / 32768.0
		}

		var samplesToSend []float32

		// Parakeet internal buffer needs 1.5-2 seconds of silence to flush final words
		// Using 1.5 seconds = 24000 samples at 16kHz as a balance
		const parakeetBufferSamples = 24000 // 1.5 seconds

		if hasSpeech {
			// Speech chunk: prepend any accumulated silence, then send
			if len(p.parakeetSilenceBuf) > 0 {
				samplesToSend = append(p.parakeetSilenceBuf, floatSamples...)
				p.parakeetSilenceBuf = nil
			} else {
				samplesToSend = floatSamples
			}
			p.parakeetWasSpeaking = true
			p.parakeetSilenceSent = false
		} else {
			// Silence chunk
			if p.parakeetSilenceSent {
				// Already sent silence flush, just accumulate
				p.parakeetSilenceBuf = append(p.parakeetSilenceBuf, floatSamples...)
				return nil
			}

			// Accumulate silence
			p.parakeetSilenceBuf = append(p.parakeetSilenceBuf, floatSamples...)

			// Check if we've accumulated 1.5 seconds of silence to flush Parakeet's buffer
			if len(p.parakeetSilenceBuf) >= parakeetBufferSamples {
				// Send accumulated silence to push Parakeet's buffer through
				samplesToSend = p.parakeetSilenceBuf
				p.parakeetSilenceBuf = nil
				p.parakeetWasSpeaking = false
				p.parakeetSilenceSent = true
			} else {
				// Not enough silence yet, keep accumulating
				return nil
			}
		}

		// Transcribe (Parakeet returns text when it has enough audio)
		text, err := p.asr.Transcribe(samplesToSend)

		// If we got text back, send it as a result
		if text != "" {
			result := TranscriptionResult{
				Text:      text,
				Timestamp: timestamp,
				Error:     err,
			}

			select {
			case p.resultChan <- result:
				p.log.InfoWithFields("Parakeet streaming result", map[string]interface{}{
					"text": text,
				})
				// Notify state tracker of transcription activity
				p.stateTracker.NotifyTranscription()
			default:
				p.log.Warn("Result dropped (channel full)")
			}
		}

		if err != nil {
			p.log.Error("Parakeet transcription error: %v", err)
		}
	} else {
		// For Whisper: use traditional VAD/chunker approach
		// The chunker will call transcribeChunk() when a chunk is ready
		p.chunker.ProcessSamples(samples)

		// Notify state tracker of VAD state from chunker
		if p.chunker.IsSpeaking() {
			p.stateTracker.NotifySpeechDetected()
		} else {
			p.stateTracker.NotifySilence()
		}
	}

	return nil
}

// transcribeChunk is called by the chunker when a chunk is ready for transcription
func (p *TranscriptionPipeline) transcribeChunk(samples []int16) {
	duration := float64(len(samples)) / 16000.0

	// Save debug WAV if enabled
	if p.debugWAV {
		p.saveDebugWAV(samples)
	}

	// Convert int16 samples to float32 for ASR engine
	floatSamples := make([]float32, len(samples))
	for i, sample := range samples {
		floatSamples[i] = float32(sample) / 32768.0
	}

	// Transcribe using ASR engine (Whisper or Parakeet)
	text, err := p.asr.Transcribe(floatSamples)

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
		// Notify state tracker of transcription activity
		p.stateTracker.NotifyTranscription()
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

	// Reset VAD and silence buffer for Parakeet
	if p.vad != nil {
		p.vad.Reset()
		p.parakeetWasSpeaking = false
		p.parakeetSilenceBuf = nil
		p.parakeetSilenceSent = false
	}

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
	engine := p.engine
	p.mu.Unlock()

	// For Parakeet streaming: send final silence to flush any remaining words
	if engine == "parakeet" {
		// Send 2 seconds of silence to ensure everything is flushed
		const flushSilenceSamples = 32000 // 2 seconds at 16kHz
		silenceSamples := make([]float32, flushSilenceSamples)

		// Send the flush silence
		text, err := p.asr.Transcribe(silenceSamples)
		if text != "" {
			result := TranscriptionResult{
				Text:      text,
				Timestamp: currentTimeMillis(),
				Error:     err,
			}
			select {
			case p.resultChan <- result:
				p.log.Info("Final Parakeet flush result: %s", text)
			default:
				p.log.Warn("Final result dropped (channel full)")
			}
		}
	} else {
		// For Whisper: use traditional chunker flush
		p.chunker.Flush()

		// Flush any remaining audio in RNNoise buffer
		remainingSamples := p.rnnoise.Flush()
		if len(remainingSamples) > 0 {
			p.chunker.ProcessSamples(remainingSamples)
			p.chunker.Flush() // Flush again after adding RNNoise remainder
		}
	}

	return nil
}

// Results returns the channel for receiving transcription results
func (p *TranscriptionPipeline) Results() <-chan TranscriptionResult {
	return p.resultChan
}

// GetRNNoise returns the RNNoise processor (for calibration endpoint)
func (p *TranscriptionPipeline) GetRNNoise() *RNNoiseProcessor {
	return p.rnnoise
}

// Close releases all resources
func (p *TranscriptionPipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.active = false

	if p.stateTracker != nil {
		p.stateTracker.Stop()
	}

	if p.asr != nil {
		p.asr.Close()
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
		Active:       p.IsActive(),
		ChunkerStats: chunkerStats,
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
	binary.Write(file, binary.LittleEndian, uint32(16))                                  // Subchunk size
	binary.Write(file, binary.LittleEndian, uint16(1))                                   // Audio format (1 = PCM)
	binary.Write(file, binary.LittleEndian, uint16(channels))                            // Number of channels
	binary.Write(file, binary.LittleEndian, uint32(sampleRate))                          // Sample rate
	binary.Write(file, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8)) // Byte rate
	binary.Write(file, binary.LittleEndian, uint16(channels*bitsPerSample/8))            // Block align
	binary.Write(file, binary.LittleEndian, uint16(bitsPerSample))                       // Bits per sample

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
