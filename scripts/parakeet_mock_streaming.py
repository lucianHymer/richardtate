#!/usr/bin/env python3
"""
Mock Parakeet Streaming Worker for Testing on Linux
Simulates the streaming protocol but returns dummy transcriptions
"""

import sys
import json
import base64
import time
import random

def decode_audio(audio_base64):
    """Decode base64 audio to get sample count"""
    audio_bytes = base64.b64decode(audio_base64)
    num_samples = len(audio_bytes) // 4  # float32 = 4 bytes
    return num_samples

def main():
    # Simulate model loading
    sys.stderr.write("Loading mock Parakeet streaming model\n")
    sys.stderr.flush()
    time.sleep(0.5)  # Simulate loading time
    sys.stderr.write("Mock streaming model loaded successfully\n")
    sys.stderr.flush()

    phrase_templates = [
        "This is streaming test transcription",
        "Mock streaming audio processed",
        "Testing the streaming protocol",
        "Streaming chunk received",
        "Simulated streaming output"
    ]

    # Track client sessions
    client_buffers = {}
    client_word_counts = {}

    while True:
        try:
            line = sys.stdin.readline()
            if not line:
                break

            request = json.loads(line)
            command = request.get('command')
            client_id = request.get('client_id')

            if command == 'start_stream':
                # Initialize streaming session for client
                client_buffers[client_id] = 0
                client_word_counts[client_id] = 0
                response = {
                    "status": "started",
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            elif command == 'add_audio':
                # Accumulate audio samples
                audio_base64 = request.get('audio', '')
                if audio_base64:
                    num_samples = decode_audio(audio_base64)
                    client_buffers[client_id] = client_buffers.get(client_id, 0) + num_samples

                # Check if we have accumulated ~1 second (16000 samples)
                text = ""
                is_final = False

                if client_buffers[client_id] >= 16000:
                    # Generate mock transcription
                    phrase = random.choice(phrase_templates)
                    word_num = client_word_counts[client_id]
                    text = f"{phrase} (chunk {word_num})"
                    client_word_counts[client_id] += 1
                    client_buffers[client_id] = 0  # Reset buffer
                    is_final = True

                response = {
                    "text": text,
                    "is_final": is_final,
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            elif command == 'end_stream':
                # Clean up client session
                if client_id in client_buffers:
                    del client_buffers[client_id]
                if client_id in client_word_counts:
                    del client_word_counts[client_id]

                response = {
                    "status": "ended",
                    "client_id": client_id
                }
                print(json.dumps(response), flush=True)

            else:
                error_response = {"error": f"Unknown command: {command}"}
                print(json.dumps(error_response), flush=True)

        except Exception as e:
            error_response = {"error": f"Mock streaming error: {str(e)}"}
            print(json.dumps(error_response), flush=True)

if __name__ == "__main__":
    main()