package transcription

import (
	"log"
	"math"
	"time"
)

// VADConfig holds configuration for Voice Activity Detection
type VADConfig struct {
	SampleRate         int     // Audio sample rate (16kHz)
	FrameDurationMs    int     // Frame duration in milliseconds (10ms)
	EnergyThreshold    float64 // Energy threshold for speech detection
	SilenceThresholdMs int     // Silence duration to trigger chunk boundary (1000ms)
}

// VoiceActivityDetector detects speech vs silence in audio
type VoiceActivityDetector struct {
	config              VADConfig
	samplesPerFrame     int
	silenceDuration     time.Duration // Accumulated silence duration
	speechDuration      time.Duration // Accumulated speech duration
	lastFrameWasSpeech  bool
	consecutiveSilence  int // Number of consecutive silent frames
	consecutiveSpeech   int // Number of consecutive speech frames
}

// NewVAD creates a new Voice Activity Detector
func NewVAD(config VADConfig) *VoiceActivityDetector {
	// Set defaults
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	if config.FrameDurationMs == 0 {
		config.FrameDurationMs = 10 // 10ms frames
	}
	if config.EnergyThreshold == 0 {
		config.EnergyThreshold = 100.0 // Tunable threshold (good for most mics)
	}
	if config.SilenceThresholdMs == 0 {
		config.SilenceThresholdMs = 1000 // 1 second
	}

	samplesPerFrame := config.SampleRate * config.FrameDurationMs / 1000

	log.Printf("[VAD] Initialized - threshold=%.0f, silence_trigger=%dms, frame_size=%d samples",
		config.EnergyThreshold, config.SilenceThresholdMs, samplesPerFrame)

	return &VoiceActivityDetector{
		config:          config,
		samplesPerFrame: samplesPerFrame,
	}
}

// ProcessFrame analyzes a single audio frame
// Returns true if speech is detected, false if silence
func (v *VoiceActivityDetector) ProcessFrame(samples []int16) bool {
	if len(samples) == 0 {
		return false
	}

	// Calculate RMS energy
	energy := v.calculateEnergy(samples)

	// Determine if speech or silence
	isSpeech := energy > v.config.EnergyThreshold

	// Log energy every 100 frames (~1 second) to help with threshold tuning
	if v.consecutiveSpeech%100 == 0 || v.consecutiveSilence%100 == 0 {
		log.Printf("[VAD] Energy: %.1f (threshold: %.1f) → %s",
			energy, v.config.EnergyThreshold, map[bool]string{true: "SPEECH", false: "SILENCE"}[isSpeech])
	}

	// Update counters
	frameDuration := time.Duration(v.config.FrameDurationMs) * time.Millisecond

	if isSpeech {
		v.consecutiveSpeech++
		v.consecutiveSilence = 0
		v.speechDuration += frameDuration
		v.silenceDuration = 0 // Reset silence counter
		v.lastFrameWasSpeech = true
	} else {
		v.consecutiveSilence++
		v.consecutiveSpeech = 0
		v.silenceDuration += frameDuration
		v.lastFrameWasSpeech = false
	}

	return isSpeech
}

// calculateEnergy computes the RMS energy of audio samples
func (v *VoiceActivityDetector) calculateEnergy(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sumSquares float64
	for _, sample := range samples {
		val := float64(sample)
		sumSquares += val * val
	}

	rms := math.Sqrt(sumSquares / float64(len(samples)))
	return rms
}

// ShouldChunk returns true if we've detected enough silence to trigger a chunk boundary
func (v *VoiceActivityDetector) ShouldChunk() bool {
	// Chunk if we've accumulated enough silence
	thresholdDuration := time.Duration(v.config.SilenceThresholdMs) * time.Millisecond
	return v.silenceDuration >= thresholdDuration
}

// GetSilenceDuration returns the current silence duration
func (v *VoiceActivityDetector) GetSilenceDuration() time.Duration {
	return v.silenceDuration
}

// GetSpeechDuration returns the accumulated speech duration since last reset
func (v *VoiceActivityDetector) GetSpeechDuration() time.Duration {
	return v.speechDuration
}

// IsSpeaking returns true if we're currently in a speech region
func (v *VoiceActivityDetector) IsSpeaking() bool {
	return v.lastFrameWasSpeech
}

// Reset clears the VAD state (useful after chunking)
func (v *VoiceActivityDetector) Reset() {
	v.silenceDuration = 0
	v.speechDuration = 0
	v.consecutiveSilence = 0
	v.consecutiveSpeech = 0
	v.lastFrameWasSpeech = false
}

// Stats returns current VAD statistics
func (v *VoiceActivityDetector) Stats() VADStats {
	return VADStats{
		SilenceDuration:    v.silenceDuration,
		SpeechDuration:     v.speechDuration,
		ConsecutiveSilence: v.consecutiveSilence,
		ConsecutiveSpeech:  v.consecutiveSpeech,
		IsSpeaking:         v.lastFrameWasSpeech,
	}
}

// VADStats holds VAD statistics
type VADStats struct {
	SilenceDuration    time.Duration
	SpeechDuration     time.Duration
	ConsecutiveSilence int
	ConsecutiveSpeech  int
	IsSpeaking         bool
}
