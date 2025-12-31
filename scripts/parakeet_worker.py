#!/usr/bin/env python3
"""
Parakeet MLX Batch Worker Process
Simple batch transcription - receives full audio chunks, returns text.

This is the chunked/batch version that works with VAD-based chunking.
Audio chunks are sent when the VAD detects silence boundaries.

Requirements:
  Install with: pip3 install -r scripts/requirements-parakeet.txt
  Or run: ./scripts/install-parakeet.sh
"""

import sys
import json
import base64
import numpy as np
import traceback
import os
import tempfile
from pathlib import Path

# Ensure common FFmpeg locations are in PATH
ffmpeg_paths = ['/opt/homebrew/bin', '/usr/local/bin', '/usr/bin']
current_path = os.environ.get('PATH', '')
for ffmpeg_path in ffmpeg_paths:
    if ffmpeg_path not in current_path and os.path.exists(ffmpeg_path):
        os.environ['PATH'] = ffmpeg_path + ':' + current_path
        current_path = os.environ['PATH']

def load_model(model_path):
    """Load Parakeet model from path"""
    from parakeet_mlx import from_pretrained

    # If model_path is a directory, assume it's a local model
    if Path(model_path).is_dir():
        return from_pretrained(model_path)
    # Otherwise, assume it's a HuggingFace model ID
    else:
        return from_pretrained(model_path)

def decode_audio(audio_base64):
    """Decode base64 audio to numpy array (float32)"""
    audio_bytes = base64.b64decode(audio_base64)
    # Convert bytes to float32 array
    audio_float32 = np.frombuffer(audio_bytes, dtype=np.float32)
    return audio_float32

def write_temp_wav(audio_samples, sample_rate=16000):
    """Write audio samples to a temporary WAV file and return the path"""
    import scipy.io.wavfile as wavfile

    # Create temporary file
    fd, temp_path = tempfile.mkstemp(suffix='.wav')
    os.close(fd)

    # Convert float32 [-1, 1] to int16
    audio_int16 = (audio_samples * 32767).astype(np.int16)

    # Write WAV file
    wavfile.write(temp_path, sample_rate, audio_int16)

    return temp_path

def transcribe_audio(model, audio_samples, sample_rate=16000):
    """Transcribe audio samples using Parakeet MLX batch mode"""
    # Write to temporary WAV file (Parakeet expects file paths)
    temp_wav_path = write_temp_wav(audio_samples, sample_rate)

    try:
        # Transcribe from file
        result = model.transcribe(temp_wav_path)
        return result.text
    finally:
        # Clean up temporary file
        try:
            os.remove(temp_wav_path)
        except:
            pass

def main():
    # Get model path from command line
    if len(sys.argv) < 2:
        print(json.dumps({"error": "Model path required"}), flush=True)
        sys.exit(1)

    model_path = sys.argv[1]

    try:
        # Load model once at startup
        sys.stderr.write(f"Loading Parakeet model from {model_path}\n")
        sys.stderr.flush()
        model = load_model(model_path)
        sys.stderr.write("Model loaded successfully\n")
        sys.stderr.flush()
    except Exception as e:
        error_msg = f"Failed to load model: {str(e)}"
        print(json.dumps({"error": error_msg}), flush=True)
        sys.exit(1)

    # Main processing loop - simple batch transcription
    while True:
        try:
            # Read line from stdin
            line = sys.stdin.readline()
            if not line:
                break  # EOF, exit gracefully

            # Parse JSON request
            request = json.loads(line)

            # Get audio from request
            audio_base64 = request.get('audio', '')
            sample_rate = request.get('sample_rate', 16000)

            # Handle empty audio (used for readiness check)
            if not audio_base64:
                response = {"text": ""}
                print(json.dumps(response), flush=True)
                continue

            # Decode audio
            audio_samples = decode_audio(audio_base64)

            # Log chunk info
            duration = len(audio_samples) / sample_rate
            sys.stderr.write(f"[Parakeet] Transcribing {duration:.1f}s chunk ({len(audio_samples)} samples)\n")
            sys.stderr.flush()

            # Transcribe
            text = transcribe_audio(model, audio_samples, sample_rate)

            # Send response
            response = {"text": text}
            print(json.dumps(response), flush=True)

        except json.JSONDecodeError as e:
            error_response = {"error": f"Invalid JSON: {str(e)}"}
            print(json.dumps(error_response), flush=True)
        except Exception as e:
            # Log full traceback to stderr for debugging
            traceback.print_exc(file=sys.stderr)
            # Send error response
            error_response = {"error": f"Transcription failed: {str(e)}"}
            print(json.dumps(error_response), flush=True)

if __name__ == "__main__":
    main()
