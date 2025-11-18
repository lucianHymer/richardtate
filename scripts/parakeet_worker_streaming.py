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
        # Note: No longer tracking previous_text since we send full text for preview

    def start_stream(self, client_id, context_size=(256, 256), depth=1):
        """Start a new streaming context for a client"""
        if client_id in self.contexts:
            # Clean up existing context first
            self.end_stream(client_id)

        # Create new streaming context
        # Use default context size from request or (256, 256)
        # NOTE: Larger context = better quality but longer finalization delay
        # With (256, 256), tokens aren't finalized for ~4 minutes
        # With (10, 10), tokens finalize after ~10 seconds
        # Future optimization: Use smaller window (e.g., 60) and only send finalized tokens

        self.contexts[client_id] = self.model.transcribe_stream(
            context_size=context_size,
            depth=depth,
            keep_original_attention=False  # Use local attention for streaming
        )
        self.contexts[client_id].__enter__()  # Start the context manager

    def add_audio(self, client_id, audio_samples):
        """Add audio to an existing streaming context"""
        if client_id not in self.contexts:
            raise ValueError(f"No streaming context for client {client_id}")

        context = self.contexts[client_id]

        # DEBUG: Print context attributes to understand the streaming object
        import sys
        sys.stderr.write(f"\n=== PARAKEET CONTEXT DEBUG ===\n")
        sys.stderr.write(f"Context type: {type(context)}\n")
        sys.stderr.write(f"Context attributes: {[attr for attr in dir(context) if not attr.startswith('_')]}\n")

        sys.stderr.write(f"=== END CONTEXT DEBUG (BEFORE add_audio) ===\n\n")
        sys.stderr.flush()

        # Add audio to the streaming context
        # Convert numpy array to MLX array - Parakeet MLX expects MLX arrays, not numpy
        mlx_audio = mx.array(audio_samples)
        context.add_audio(mlx_audio)

        # NOW check for finalized/draft tokens AFTER adding audio
        sys.stderr.write(f"\n=== TOKENS AFTER add_audio ===\n")

        # These should exist based on the Parakeet streaming API
        if hasattr(context, 'finalized_tokens'):
            sys.stderr.write(f"context.finalized_tokens exists: {context.finalized_tokens}\n")
            sys.stderr.write(f"Type: {type(context.finalized_tokens)}\n")
            # Try to get text from finalized tokens
            if context.finalized_tokens:
                sys.stderr.write(f"Finalized text length: {len(str(context.finalized_tokens))}\n")
        else:
            sys.stderr.write(f"NO finalized_tokens attribute!\n")

        if hasattr(context, 'draft_tokens'):
            sys.stderr.write(f"context.draft_tokens exists: {context.draft_tokens}\n")
            sys.stderr.write(f"Type: {type(context.draft_tokens)}\n")
            if context.draft_tokens:
                sys.stderr.write(f"Draft text length: {len(str(context.draft_tokens))}\n")
        else:
            sys.stderr.write(f"NO draft_tokens attribute!\n")

        sys.stderr.write(f"=== END TOKENS ===\n\n")
        sys.stderr.flush()

        # Get current result (includes both finalized and draft tokens)
        result = context.result

        # DEBUG: Print AlignedResult details
        sys.stderr.write(f"\n=== PARAKEET RESULT DEBUG ===\n")
        sys.stderr.write(f"Result type: {type(result).__name__}\n")
        sys.stderr.write(f"Full text: '{result.text}'\n")
        sys.stderr.write(f"Num sentences: {len(result.sentences) if hasattr(result, 'sentences') else 0}\n")

        # Show text length to track growth
        sys.stderr.write(f"Text length: {len(result.text)} chars\n")

        # Show if we have finalized tokens (for future optimization)
        if hasattr(context, 'finalized_tokens') and context.finalized_tokens:
            sys.stderr.write(f"Finalized tokens exist (could use for incremental mode)\n")
        else:
            sys.stderr.write(f"No finalized tokens yet (using full preview mode)\n")

        sys.stderr.write(f"=== END DEBUG ===\n\n")
        sys.stderr.flush()

        # Extract the FULL text from the result (entire accumulated transcription)
        full_text = result.text if hasattr(result, 'text') else str(result)

        # For preview mode: Always send the FULL text
        # Client will completely replace the preview each time
        # This allows Parakeet to revise earlier text as it gets more context

        # Note: We're not using finalized_tokens yet because with default
        # context_size=(256,256) they take 4+ minutes to finalize.
        # Future optimization could use smaller window and track finalized vs draft.

        return {
            "text": full_text,  # Full current transcription (may revise)
            "is_final": False,  # Always false during streaming
            "client_id": client_id,
            "is_preview": True  # Indicate this is preview text that may change
        }

    def end_stream(self, client_id):
        """End a streaming context for a client"""
        if client_id in self.contexts:
            try:
                self.contexts[client_id].__exit__(None, None, None)
            except:
                pass  # Context might already be closed
            del self.contexts[client_id]

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