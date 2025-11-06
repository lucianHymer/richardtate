### [19:22] [architecture] Hammerspoon Calibration UI API Architecture
**Details**: Implemented a clean separation of concerns for VAD calibration:

**Client API Endpoints** (stateless):
- POST /api/calibrate/record - Records audio for N seconds, returns energy stats (doesn't know if background or speech)
- POST /api/calibrate/calculate - Takes background + speech stats, calculates threshold (P95 × 1.5 logic in Go)
- POST /api/calibrate/save - Saves threshold to client config YAML

**Architecture Benefits**:
1. Stateless recording endpoint - Hammerspoon decides what phase it is
2. Server-side calculation - Threshold logic stays in Go (testable, consistent with CLI wizard)
3. Clean separation - Hammerspoon handles UI/UX, client API handles business logic
4. No restart needed - Calibrate anytime while daemon is running

**API Constructor Change**:
api.New() now requires *config.Config parameter (needed for server URL conversion and audio device config)

**Files**:
- client/internal/api/server.go - Added ~300 lines for calibration endpoints
- client/cmd/client/main.go - Updated api.New() call to pass config
**Files**: client/internal/api/server.go, client/cmd/client/main.go
---

### [19:23] [frontend] Hammerspoon Calibration UI Implementation
**Details**: Created a complete visual calibration wizard in Hammerspoon using canvas API:

**UI Design** (hammerspoon/calibration.lua - 450 lines):
- 3-step wizard: Background → Speech → Results
- Canvas-based UI (500x400px floating window)
- Dark theme matching macOS aesthetic
- Real-time recording progress indicators (updates every 0.5s)
- Visual energy comparison bars
- Click-based interaction (canvas mouse events)

**User Flow**:
1. Press Ctrl+Alt+C → Wizard opens
2. Step 1 (Blue theme): "Stay silent" → Record 5s → Background stats
3. Step 2 (Orange theme): "Speak normally" → Record 5s → Speech stats
4. Step 3 (Green theme): Visual bars → Recommended threshold → Save/Cancel buttons

**Integration**:
- Hotkey: Ctrl+Alt+C (configurable)
- Module-based: require("calibration") in init.lua
- Error handling with macOS notifications
- Cleanup on reload (calibration.close())

**Why Canvas (Not WebView)**:
- Simpler: Pure Lua drawing (~450 lines vs HTML+CSS+JS)
- Faster: No browser engine
- Native: Matches macOS look
- Lightweight: Minimal dependencies

**Files**:
- hammerspoon/calibration.lua - Complete wizard implementation
- hammerspoon/init.lua - Integration with Ctrl+Alt+C hotkey
**Files**: hammerspoon/calibration.lua, hammerspoon/init.lua
---

