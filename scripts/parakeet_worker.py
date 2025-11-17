#!/usr/bin/env python3
"""
Parakeet MLX Worker Process
Reads audio from stdin, transcribes with Parakeet, writes to stdout

Requirements:
  Install with: pip3 install -r scripts/requirements-parakeet.txt
  Or run: ./scripts/install-parakeet.sh
"""

import sys
import json
import base64
import numpy as np
import traceback
import tempfile
import os
from pathlib import Path
from scipy.io import wavfile

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

def decode_audio(audio_base64, sample_rate=16000):
    """Decode base64 audio to numpy array"""
    audio_bytes = base64.b64decode(audio_base64)
    # Convert bytes to float32 array
    audio_float32 = np.frombuffer(audio_bytes, dtype=np.float32)
    return audio_float32

def write_temp_wav(audio_samples, sample_rate=16000):
    """Write audio samples to a temporary WAV file and return the path"""
    # Create a temporary file that won't be automatically deleted
    fd, temp_path = tempfile.mkstemp(suffix='.wav')
    os.close(fd)  # Close the file descriptor, we'll use wavfile.write

    # Convert float32 [-1, 1] to int16 for WAV file
    audio_int16 = (audio_samples * 32767).astype(np.int16)

    # Write WAV file
    wavfile.write(temp_path, sample_rate, audio_int16)

    return temp_path

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

    # Main processing loop
    while True:
        try:
            # Read line from stdin
            line = sys.stdin.readline()
            if not line:
                break  # EOF, exit gracefully

            # Parse JSON request
            request = json.loads(line)
            audio_base64 = request['audio']
            sample_rate = request.get('sample_rate', 16000)

            # Decode audio
            audio_samples = decode_audio(audio_base64, sample_rate)

            # Write to temporary WAV file
            temp_wav_path = write_temp_wav(audio_samples, sample_rate)

            try:
                # Transcribe from file
                result = model.transcribe(temp_wav_path)

                # Send response (result is an AlignedResult object with .text attribute)
                response = {"text": result.text}
                print(json.dumps(response), flush=True)
            finally:
                # Clean up temporary file
                if os.path.exists(temp_wav_path):
                    os.remove(temp_wav_path)

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
