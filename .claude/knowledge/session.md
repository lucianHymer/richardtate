### [22:01] [architecture] Parakeet Finalized Tokens and Context Window Behavior
**Details**: Critical discovery about Parakeet streaming finalized tokens:

CONTEXT WINDOW SIZE DIRECTLY AFFECTS FINALIZATION DELAY:
- context_size=(256, 256) means 256 frames lookahead/lookbehind
- At 16kHz with ~62.5 frames/second, 256 frames = ~4 seconds lookahead
- Tokens only become "finalized" after passing through the lookahead window
- This means with (256, 256), tokens aren't finalized for 4+ MINUTES of audio!

TESTED BEHAVIOR:
- With context_size=(10, 10): Tokens finalize after ~10 seconds
- With context_size=(256, 256): Tokens don't finalize for 4+ minutes
- result.text contains BOTH finalized AND draft tokens combined
- finalized_tokens grows over time as tokens pass the lookahead window
- Text constantly revises until finalized

FUTURE OPTIMIZATION OPTIONS:
1. Reduce context window to ~60 frames (~1 second lookahead)
   - Faster finalization, slight quality tradeoff
   - Send only finalized tokens for incremental updates
   - Show draft tokens only in preview

2. Dual-mode approach:
   - Use finalized tokens for "committed" text
   - Show full result.text in preview window
   - User sees both stable and evolving text

3. Adaptive window:
   - Start with small window for responsiveness
   - Increase window size during pauses for quality

CURRENT APPROACH (for simplicity):
- Use default (256, 256) for maximum quality
- Send full result.text as preview (includes all tokens)
- Accept that text will revise continuously
- Only commit final text when streaming ends

This explains why Parakeet never seemed to finalize - we were using such a large context window that finalization took longer than most test recordings!
**Files**: scripts/parakeet_worker_streaming.py
---

