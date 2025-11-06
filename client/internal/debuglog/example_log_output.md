# Debug Log Example Output

This shows what the debug log looks like in practice.

## Example Session

### Chunk Entries (as transcriptions arrive)
```json
{"timestamp":"2025-11-06T16:30:45.123456Z","type":"chunk","text":"Hello, this is a test","chunk_id":1}
{"timestamp":"2025-11-06T16:30:47.234567Z","type":"chunk","text":"of the streaming transcription system","chunk_id":2}
{"timestamp":"2025-11-06T16:30:49.345678Z","type":"chunk","text":"It's working really well","chunk_id":3}
```

### Complete Entry (when recording stops)
```json
{"timestamp":"2025-11-06T16:30:51.456789Z","type":"complete","full_text":"Hello, this is a test of the streaming transcription system It's working really well","duration_seconds":6.3}
```

### Inserted Entry (when text is inserted - V2 feature)
```json
{"timestamp":"2025-11-06T16:30:52.567890Z","type":"inserted","location":"Obsidian","length":87}
```

## Log File Location

Default: `./debug.log` (configurable in `config.yaml`)

## Recovery Example

If the UI crashes, you can recover your transcription:

```bash
# View all chunks from last session
jq 'select(.type=="chunk")' debug.log | tail -20

# Get complete text from last session
jq -r 'select(.type=="complete") | .full_text' debug.log | tail -1

# Search for specific content
jq -r 'select(.text | contains("important keyword"))' debug.log
```

## Log Rotation

When `debug.log` reaches 8MB:
1. Existing `debug.log.1` is deleted (if exists)
2. Current `debug.log` is renamed to `debug.log.1`
3. New empty `debug.log` is created

This gives you ~16MB total (8MB current + 8MB archived).

At 500k+ words capacity, this is several hours of continuous dictation.

## Features

- **Sync writes**: Each chunk is flushed immediately (safety)
- **JSON format**: Easy to parse and search
- **Timestamped**: UTC timestamps with nanosecond precision
- **Sequential IDs**: Chunk IDs for ordering/debugging
- **Session tracking**: Complete sessions logged with duration
- **Automatic rotation**: No manual cleanup needed
