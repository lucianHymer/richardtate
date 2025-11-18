#!/usr/bin/env python3
"""
Parakeet MLX Streaming Worker Process
Maintains streaming contexts per client for real-time transcription

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
from pathlib import Path
import mlx.core as mx

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

class StreamingManager:
    """Manages streaming contexts for multiple clients"""

    def __init__(self, model):
        self.model = model
        self.contexts = {}  # client_id -> streaming context
        self.previous_text = {}  # client_id -> previously sent text

    def start_stream(self, client_id, context_size=(256, 256), depth=1):
        """Start a new streaming context for a client"""
        if client_id in self.contexts:
            # Clean up existing context first
            self.end_stream(client_id)

        # Create new streaming context
        self.contexts[client_id] = self.model.transcribe_stream(
            context_size=context_size,
            depth=depth,
            keep_original_attention=False  # Use local attention for streaming
        )
        self.contexts[client_id].__enter__()  # Start the context manager
        # Initialize previous text tracker
        self.previous_text[client_id] = ""

    def add_audio(self, client_id, audio_samples):
        """Add audio to an existing streaming context"""
        if client_id not in self.contexts:
            raise ValueError(f"No streaming context for client {client_id}")

        context = self.contexts[client_id]

        # Add audio to the streaming context
        # Convert numpy array to MLX array - Parakeet MLX expects MLX arrays, not numpy
        mlx_audio = mx.array(audio_samples)
        context.add_audio(mlx_audio)

        # Get current result (includes both finalized and draft tokens)
        result = context.result

        # DEBUG: Print full result object to see what we're getting
        import sys
        sys.stderr.write(f"\n=== PARAKEET RESULT DEBUG ===\n")
        sys.stderr.write(f"Result type: {type(result)}\n")
        sys.stderr.write(f"Result attributes: {dir(result)}\n")

        # Try to access various attributes
        if hasattr(result, 'text'):
            sys.stderr.write(f"result.text: '{result.text}'\n")
        if hasattr(result, 'finalized'):
            sys.stderr.write(f"result.finalized: '{result.finalized}'\n")
            sys.stderr.write(f"result.finalized type: {type(result.finalized)}\n")
        if hasattr(result, 'draft'):
            sys.stderr.write(f"result.draft: '{result.draft}'\n")

        # Print the full object representation
        sys.stderr.write(f"Full result str(): '{str(result)}'\n")
        sys.stderr.write(f"Full result repr(): '{repr(result)}'\n")
        sys.stderr.write(f"=== END DEBUG ===\n\n")
        sys.stderr.flush()

        # Extract the FULL text from the result (entire accumulated transcription)
        full_text = result.text if hasattr(result, 'text') else str(result)

        # Calculate the incremental text (what's new since last time)
        previous = self.previous_text.get(client_id, "")
        incremental_text = ""

        # Only send the new part that hasn't been sent before
        if len(full_text) > len(previous):
            incremental_text = full_text[len(previous):]
            # Update the previous text tracker
            self.previous_text[client_id] = full_text

        # Check if we have finalized tokens (stable) vs draft tokens (may change)
        is_final = hasattr(result, 'finalized') and len(result.finalized) > 0

        return {
            "text": incremental_text,  # Only send the new/incremental text
            "is_final": is_final,
            "client_id": client_id
        }

    def end_stream(self, client_id):
        """End a streaming context for a client"""
        if client_id in self.contexts:
            try:
                self.contexts[client_id].__exit__(None, None, None)
            except:
                pass  # Context might already be closed
            del self.contexts[client_id]
        # Clean up previous text tracker
        if client_id in self.previous_text:
            del self.previous_text[client_id]

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

    # Create streaming manager
    manager = StreamingManager(model)

    # Main processing loop
    while True:
        try:
            # Read line from stdin
            line = sys.stdin.readline()
            if not line:
                break  # EOF, exit gracefully

            # Parse JSON request
            request = json.loads(line)
            command = request.get('command')
            client_id = request.get('client_id')

            if command == 'start_stream':
                # Start a new streaming context
                context_size = request.get('context_size', [256, 256])
                depth = request.get('depth', 1)
                manager.start_stream(client_id, tuple(context_size), depth)
                response = {
                    "status": "started",
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            elif command == 'add_audio':
                # Add audio to existing stream
                audio_base64 = request['audio']
                sample_rate = request.get('sample_rate', 16000)

                # Decode audio
                audio_samples = decode_audio(audio_base64, sample_rate)

                # Process through streaming context
                result = manager.add_audio(client_id, audio_samples)

                # Send response with transcription
                response = {
                    "text": result["text"],
                    "is_final": result["is_final"],
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            elif command == 'end_stream':
                # End streaming context
                manager.end_stream(client_id)
                response = {
                    "status": "ended",
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            else:
                error_response = {"error": f"Unknown command: {command}"}
                print(json.dumps(error_response), flush=True)

        except json.JSONDecodeError as e:
            error_response = {"error": f"Invalid JSON: {str(e)}"}
            print(json.dumps(error_response), flush=True)
        except Exception as e:
            # Log full traceback to stderr for debugging
            traceback.print_exc(file=sys.stderr)
            # Send error response
            error_response = {"error": f"Processing failed: {str(e)}"}
            print(json.dumps(error_response), flush=True)

if __name__ == "__main__":
    main()