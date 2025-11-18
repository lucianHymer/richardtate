# Parakeet Streaming Preview Window Design

**Status**: Design Proposal
**Last Updated**: 2025-11-18

## Overview
Parakeet streaming never populates finalized_tokens and constantly revises the entire transcription as more context arrives. This makes incremental text sending impossible and requires a different approach for real-time display.

## Problem Statement
Unlike traditional streaming ASR which provides incremental finalized text, Parakeet's streaming API:
- Returns the entire accumulated transcription on each update
- Can revise earlier portions as more context arrives
- Never marks tokens as finalized
- Makes traditional incremental text display impossible

## Proposed Solution: Preview Window Approach
Display the full transcription as a replaceable preview that users can see updating in real-time, with explicit commit actions to finalize text.

## Design Options

### 1. Full Replacement Protocol
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