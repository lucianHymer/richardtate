# Whisper Initial Prompt Support

**Last Updated**: 2025-11-06 (Session 16)

## Overview
Whisper.cpp supports initial prompts through the SetInitialPrompt() method, allowing context to be provided to improve transcription accuracy for domain-specific vocabulary or scenarios.

## Technical Details

### API Interface
The whisper Context interface provides:
```go
SetInitialPrompt(prompt string)
```

### How It Works
1. Initial prompts are converted to tokens and provided to the decoder as context
2. Maximum of `whisper_n_text_ctx()/2` tokens are used (typically 224 tokens)
3. The prompt is prepended to any existing text context from previous calls
4. There's also a `carry_initial_prompt` flag that can prepend the initial prompt to every decode window

### Implementation Location
- **File**: `server/internal/transcription/whisper.go`
- **Current Status**: Context is created and configured but SetInitialPrompt() is not currently being called
- **Interface Definition**: `deps/whisper.cpp/bindings/go/pkg/whisper/interface.go`
- **C Header**: `deps/whisper.cpp/include/whisper.h`

## Use Cases

### Domain-Specific Vocabulary
Provide technical terms, product names, or jargon specific to the use case:
```go
ctx.SetInitialPrompt("WebRTC, Whisper.cpp, RNNoise, VAD, transcription pipeline")
```

### Conversation Context
Prime the model with conversation style or format:
```go
ctx.SetInitialPrompt("Technical discussion about software engineering and system architecture.")
```

### Command Examples
From whisper.cpp examples:
```go
// Chess moves example
ctx.SetInitialPrompt("bishop to c3, rook to d4, knight to e5...")

// Command context
ctx.SetInitialPrompt(context.data()) // Uses previous context as initial prompt
```

## Benefits

### Improved Accuracy
- Better recognition of domain-specific terms
- Reduced hallucinations on technical vocabulary
- More consistent formatting

### Context Preservation
- Can carry context between transcription chunks
- Helps maintain coherent output across segments

### Customization
- Allows per-session or per-user customization
- Can adapt to different use cases dynamically

## Potential Implementation

### Static Context
Add a fixed initial prompt for technical transcription:
```go
// In whisper.go NewWhisperTranscriber or Transcribe method
ctx.SetInitialPrompt("Technical transcription for software development. Terms include: API, WebRTC, JSON, YAML, function, variable, class, method, implementation.")
```

### Dynamic Context
Allow initial prompt to be configured:
```go
type WhisperConfig struct {
    ModelPath     string
    InitialPrompt string  // New field
}

// In Transcribe method
if w.config.InitialPrompt != "" {
    ctx.SetInitialPrompt(w.config.InitialPrompt)
}
```

### Per-Client Context
Different prompts for different clients based on their use case:
```go
// In pipeline creation
whisperConfig := WhisperConfig{
    ModelPath: modelPath,
    InitialPrompt: getPromptForClient(clientID),
}
```

## Considerations

### Token Limit
- Limited to ~224 tokens (roughly 150-200 words)
- Longer prompts will be truncated
- Need to be concise but comprehensive

### Performance Impact
- Minimal overhead for short prompts
- May slightly increase processing time for maximum-length prompts
- Worth testing for specific use cases

### Prompt Engineering
- Requires experimentation to find effective prompts
- Too specific may bias output incorrectly
- Too general may not provide benefit

## Future Enhancements

### Configurable Prompts
Add to client configuration:
```yaml
transcription:
  whisper_initial_prompt: "Technical discussion, software engineering terms"
```

### Context Accumulation
Build context from previous transcriptions:
```go
// Accumulate key terms from session
var contextTerms []string
// ... collect important terms ...
ctx.SetInitialPrompt(strings.Join(contextTerms, ", "))
```

### Adaptive Prompting
Adjust prompt based on detected content:
```go
if detectingCodeDiscussion {
    ctx.SetInitialPrompt("Code review discussion with function and variable names")
} else if detectingMeeting {
    ctx.SetInitialPrompt("Team meeting discussion with action items")
}
```

## Testing Approach

To test initial prompt effectiveness:
1. Create test set with domain-specific terms
2. Transcribe without initial prompt (baseline)
3. Transcribe with various initial prompts
4. Compare accuracy metrics
5. Identify optimal prompt for use case

## References
- Whisper.cpp documentation on initial prompts
- Example usage in whisper.cpp/examples/
- Go bindings interface documentation

## Related Systems
- [Whisper and RNNoise Setup](whisper-and-rnnoise-setup.md) - Main Whisper integration
- [Transcription Pipeline](../architecture/transcription-pipeline.md) - Where Whisper is used