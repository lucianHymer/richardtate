#!/bin/bash
# Test script to verify multi-client support with different VAD settings

echo "Multi-Client VAD Test"
echo "===================="
echo ""
echo "This script demonstrates that multiple clients can connect"
echo "with different VAD settings and each gets their own pipeline."
echo ""

# Create two test client configs with different VAD settings
cat > /tmp/client1.yaml << EOF
client:
  api_bind_address: "localhost:8081"
  debug: true
  debug_log_path: "/tmp/client1-debug.log"

server:
  url: "ws://localhost:8080"

audio:
  device_name: ""

transcription:
  vad:
    energy_threshold: 100.0      # Low threshold - sensitive
    silence_threshold_ms: 500     # Quick chunks
    min_chunk_duration_ms: 300
    max_chunk_duration_ms: 10000
EOF

cat > /tmp/client2.yaml << EOF
client:
  api_bind_address: "localhost:8082"  # Different port!
  debug: true
  debug_log_path: "/tmp/client2-debug.log"

server:
  url: "ws://localhost:8080"

audio:
  device_name: ""

transcription:
  vad:
    energy_threshold: 500.0       # High threshold - less sensitive
    silence_threshold_ms: 2000    # Longer silence needed
    min_chunk_duration_ms: 1000
    max_chunk_duration_ms: 30000
EOF

echo "Created test configs:"
echo "  Client 1: Low threshold (100), quick chunks (500ms silence)"
echo "  Client 2: High threshold (500), slow chunks (2000ms silence)"
echo ""
echo "To test:"
echo ""
echo "1. Start the server:"
echo "   cd server && ./server"
echo ""
echo "2. In another terminal, start Client 1:"
echo "   cd client && ./client --config /tmp/client1.yaml"
echo ""
echo "3. In another terminal, start Client 2:"
echo "   cd client && ./client --config /tmp/client2.yaml"
echo ""
echo "4. Use the API to start recording on both:"
echo "   curl -X POST http://localhost:8081/start  # Client 1"
echo "   curl -X POST http://localhost:8082/start  # Client 2"
echo ""
echo "5. Watch the server logs - you should see:"
echo "   - Different VAD thresholds for each client"
echo "   - Independent pipeline creation"
echo "   - Different chunking behavior"
echo ""
echo "Test configs saved to /tmp/client1.yaml and /tmp/client2.yaml"