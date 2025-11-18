# Parakeet Streaming Preview Window Design

**Status**: Implementation Complete with Context Window Discovery
**Last Updated**: 2025-11-18

## Overview
Parakeet streaming uses a context window that directly affects when tokens become finalized. With default settings, tokens may not finalize for several minutes, requiring a preview-based approach for real-time display.

## Problem Statement
Parakeet's streaming behavior is controlled by its context window size:
- Context window size determines lookahead/lookbehind in frames
- Default (256, 256) means 256 frames lookahead = ~4 seconds at 16kHz
- Tokens only become "finalized" after passing through the lookahead window
- With default settings, tokens don't finalize for 4+ minutes of audio!
- `result.text` contains both finalized and draft tokens combined
- Text constantly revises until tokens are finalized

## Proposed Solution: Preview Window Approach
Display the full transcription as a replaceable preview that users can see updating in real-time, with explicit commit actions to finalize text.

## Context Window Behavior Discovery

### Tested Configurations
- **context_size=(10, 10)**: Tokens finalize after ~10 seconds
- **context_size=(256, 256)**: Tokens don't finalize for 4+ minutes
- At 16kHz with ~62.5 frames/second, 256 frames = ~4 seconds lookahead
- Finalization delay = lookahead window duration

### Current Implementation
Using default (256, 256) for maximum quality:
- Send full `result.text` as preview (includes all tokens)
- Accept that text will revise continuously
- Only commit final text when streaming ends
- Simplest approach, highest quality

### Future Optimization Options

#### 1. Reduced Context Window
- Use ~60 frames (~1 second lookahead)
- Faster finalization, slight quality tradeoff
- Send only finalized tokens for incremental updates
- Show draft tokens only in preview

#### 2. Dual-Mode Approach
- Use finalized tokens for "committed" text
- Show full result.text in preview window
- User sees both stable and evolving text
- Best of both worlds

#### 3. Adaptive Window
- Start with small window for responsiveness
- Increase window size during pauses for quality
- Dynamic adjustment based on speech patterns

## Design Options (Original)

### 1. Full Replacement Protocol (IMPLEMENTED)
- Each update replaces entire preview
- Simple to implement
- May cause visual jumps

### 2. Revision Tracking
- Send diffs showing what changed
- More complex but smoother UX
- Requires diff algorithm

### 3. Dual Display
- Separate preview and committed panes
- Clear distinction between draft and final
- More screen real estate needed

### 4. Confidence-Based Display
- Use AlignedResult confidence scores
- Highlight low-confidence portions
- Visual indication of stability

### 5. Time-Based Stabilization
- Auto-commit after X seconds of no changes
- Balance between responsiveness and stability
- Configurable timeout

## Recommended Approach
**Combine dual display with time-based stabilization**:
- Show live preview that updates constantly
- Auto-commit after 2-3 seconds of stability
- Provide manual commit option
- Clear visual distinction between preview and committed text

## Implementation Requirements

### Protocol Changes
- New message types for preview updates
- Separate types for preview vs committed text
- Preview replacement/update messages

### ASR Interface Extensions
- Preview methods in addition to standard transcribe
- State management for preview buffers
- Stability detection logic

### Client-Side Changes
- Preview buffer management
- Visual distinction in UI
- Commit trigger handling
- State synchronization

## Benefits
- Real-time feedback during speech
- Accurate final transcription with full context
- User control over when text is finalized
- Works with Parakeet's revision model

## Trade-offs
- More complex than simple incremental display
- Requires UI changes for preview display
- Additional state management overhead
- May feel different from traditional streaming

## Related Systems
- [Parakeet Integration](parakeet-integration.md) - Main Parakeet implementation
- [ASR Interface Abstraction](asr-interface-abstraction.md) - Interface that needs extension

## Files
- `scripts/parakeet_worker_streaming.py` - Python worker implementation
- `server/internal/transcription/parakeet_shared.go` - Go side implementation