#!/usr/bin/env python3
"""
Mock Parakeet Worker for Testing on Linux
Simulates the same protocol but returns dummy transcriptions
"""

import sys
import json
import base64
import time
import random

def main():
    # Simulate model loading
    sys.stderr.write("Loading mock Parakeet model\n")
    sys.stderr.flush()
    time.sleep(1)  # Simulate loading time
    sys.stderr.write("Mock model loaded successfully\n")
    sys.stderr.flush()

    phrase_templates = [
        "This is a test transcription",
        "Mock audio processed successfully",
        "Testing the subprocess communication",
        "Audio chunk received and processed",
        "Simulated transcription output"
    ]

    while True:
        try:
            line = sys.stdin.readline()
            if not line:
                break

            request = json.loads(line)
            audio_base64 = request['audio']

            # Decode to get audio length
            audio_bytes = base64.b64decode(audio_base64)
            num_samples = len(audio_bytes) // 4  # float32 = 4 bytes
            duration = num_samples / 16000.0  # Assuming 16kHz

            # Simulate processing time (roughly proportional to audio length)
            time.sleep(min(duration * 0.3, 2.0))  # 30% of audio duration, max 2s

            # Generate mock transcription
            text = random.choice(phrase_templates)
            text += f" [{duration:.1f}s of audio]"

            response = {"text": text}
            print(json.dumps(response), flush=True)

        except Exception as e:
            error_response = {"error": f"Mock error: {str(e)}"}
            print(json.dumps(error_response), flush=True)

if __name__ == "__main__":
    main()
