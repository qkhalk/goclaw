# Plan: Tools UI Redesign + Clouds Fixes

## Overview
5 tasks, 3 parallel subagents + main agent. File boundaries strictly enforced — zero overlap.

**Sau này cook:** `$ak:cook plans/260914-tools-redesign-clouds-fixes/plan.md --parallel`

---

## Task 1: Fix Storyboard Validation Bug (Main Agent)
**Problem:** "source is required for video scenes" when creating render job — default scene is type="image" with empty source, client doesn't validate before submit.

**Fix:**
1. Add validation function:
```typescript
function hasValidationErrors(sb: Storyboard): boolean {
  return sb.scenes.some(
    (s) => (s.type === "image" || s.type === "video") && !s.source?.trim()
  );
}
```
2. Update submit button: `disabled={submitting || totalSec <= 0 || hasValidationErrors(sb)}`
3. Add red border on source input when empty for image/video scenes

**File:** `ui/web/src/pages/tools/video/video-tool-page.tsx` (validation fix ONLY)

---

## Task 2: Restructure TOOLS Sidebar (Main Agent)
Split single TOOLS group into direct links:
- **Video Editor** → `/tools/video` (Clapperboard icon)
- **Tools Hub** → `/tools` (Wrench icon)
- **Watermark Remover** → `/tools/watermark` (Eraser icon)

**Files:**
- `ui/web/src/components/layout/sidebar.tsx`
- `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/sidebar.json` (add nav.videoEditor, nav.watermarkRemover)

---

## Task 3: Watermark Remover Redesign + Video Support (Agent B)
Redesign to modern layout matching reference sites. Add VIDEO watermark removal for short clips (<30s).

### New Layout Structure
1. **Hero Section** — title, description, privacy badge
2. **Tabs** — "Images" | "Videos"
3. **Upload Area** — drag-drop zone with file type indicators
4. **Before/After Comparison Slider** — interactive CSS clip-path slider per item
5. **Processing Queue** — per-item status with progress
6. **Batch Download** — download all processed files

### Video Watermark Removal
- Client-side only via HTMLVideoElement + Canvas API
- Decode frames → apply watermark removal → re-encode via WebCodecs
- Limit: 30 seconds max, 100MB max file size
- Progress: show frame X/Y during processing

### Files
- REWRITE: `ui/web/src/pages/tools/watermark/watermark-tool-page.tsx`
- NEW: `ui/web/src/pages/tools/watermark/hooks/use-video-watermark.ts`
- Toolbox i18n: `watermark.*` keys in all 5 locale JSON files

### Key i18n Keys (add to toolbox.json)
```
watermark.tabs.images / watermark.tabs.videos
watermark.video.title / watermark.video.max_duration / watermark.video.max_size
watermark.video.processing_frame / watermark.video.not_supported
watermark.comparison.before / watermark.comparison.after / watermark.comparison.drag_to_compare
```

---

## Task 4: Video Render Client-Side Redesign (Agent C)
Canvas player preview + Timeline editor + TTS voice + WebCodecs export + overload detection.

### Architecture

#### Canvas Player (`use-canvas-player.ts` + `canvas-player.tsx`)
- Pure Canvas 2D API rendering (no heavy deps needed)
- Play/pause, seek, frame-by-frame preview
- Render active scene (image/color/video) with Ken Burns effects + captions
- `requestAnimationFrame` loop at storyboard FPS

#### Timeline Editor (`use-timeline.ts` + `timeline.tsx` + `scene-card.tsx`)
- Horizontal scrollable scene cards with thumbnails
- Drag-reorder scenes (native HTML5 drag or @dnd-kit)
- Select scene to edit in detail panel
- Undo/redo via history stack
- Scene duration display

#### TTS Integration
- **Preview:** Web Speech API (browser-native, instant, free)
- **Final voice:** POST to existing backend TTS endpoint (Edge TTS / OpenAI / ElevenLabs)
- Per-scene narration with voice selection dropdown
- Narration field in scene editor

#### Export (`use-video-export.ts` + `render-panel.tsx`)
- **Client-side:** WebCodecs API encode → MP4 download
- **Server fallback:** POST /v1/video/jobs (existing endpoint)
- **Overload detection:**
  - `navigator.hardwareConcurrency` (CPU cores)
  - `navigator.deviceMemory` (RAM in GB)
  - If cores < 8 OR memory < 4 → recommend server
  - Show badge: "Client OK" / "Server Recommended"

### New File Structure
```
ui/web/src/pages/tools/video/
├── video-tool-page.tsx              (REWRITE: tabs editor + render)
├── hooks/
│   ├── use-video.ts                 (existing API hooks — keep)
│   ├── use-canvas-player.ts         (NEW: canvas rendering engine)
│   ├── use-timeline.ts              (NEW: scene ordering, undo/redo)
│   └── use-video-export.ts          (NEW: WebCodecs export + overload detection)
├── components/
│   ├── canvas-player.tsx            (NEW: video preview canvas)
│   ├── timeline.tsx                 (NEW: timeline with scene cards)
│   ├── scene-card.tsx               (NEW: individual scene editor)
│   └── render-panel.tsx             (NEW: export settings + progress)
```

### Key i18n Keys (add to toolbox.json)
```
video.tabs.editor / video.tabs.render
video.canvas.play / video.canvas.pause / video.canvas.time
video.timeline.title / video.timeline.undo / video.timeline.redo
video.scene.n / video.scene.duration
video.render_panel.title / video.render_panel.client_method / video.render_panel.server_method
video.render_panel.client_ok / video.render_panel.server_recommended
video.render_panel.hardware_info / video.render_panel.render_button
video.tts.voice / video.tts.preview / video.tts.generate
```

---

## Task 5: Clouds UI Fixes (Agent A)
Fix layout issues, icon styling, file preview, and add settings.

### Fixes
1. **"Ổ của tôi" positioning** — should be inside the rail section, not as separate header outside
2. **"Bộ nhớ" (Storage) position** — pushed too high near Mail, should be at bottom of rail with `mt-auto`
3. **Folder icon border too thick** — add `strokeWidth={1.5}` to all Folder icons
4. **File preview icons** — add file type badge overlay (extension label) on thumbnails for non-image files
5. **Preview settings** — add section in settings-modal for thumbnail size + show hidden files

### Files
- `ui/web/src/pages/cloud/drive/drive-rail.tsx` (layout fix: "Ổ của tôi" inside, "Bộ nhớ" at bottom)
- `ui/web/src/pages/cloud/drive/drive-item.tsx` (icon stroke fix)
- `ui/web/src/pages/cloud/drive/file-thumbnail.tsx` (badge overlay + icon stroke)
- `ui/web/src/pages/cloud/settings-modal.tsx` (preview settings section)
- Cloud i18n: `settings.preview / settings.thumbnail_size / settings.small / settings.medium / settings.large / settings.show_hidden` in `cloud.json` (5 locales)

---

## Subagent File Allocation (ZERO overlap)

| Agent | Owns (touch ONLY these files) |
|-------|------|
| **Main** | `sidebar.tsx`, `sidebar.json` (×5), `video-tool-page.tsx` (validation fix line changes only), final `toolbox.json` merge |
| **A (Clouds)** | `drive-rail.tsx`, `drive-item.tsx`, `file-thumbnail.tsx`, `settings-modal.tsx`, `cloud.json` (×5) |
| **B (Watermark)** | `watermark-tool-page.tsx` (rewrite), NEW `hooks/use-video-watermark.ts` |
| **C (Video)** | `video-tool-page.tsx` (full rewrite), 3 NEW hooks, 4 NEW components |

### i18n Coordination
- Each agent defines its i18n keys in a code block in its output
- Main agent merges all toolbox.json at the end (single merge step)
- Agents MUST NOT write to toolbox.json directly — only provide their key additions

---

## Execution Order
1. Spawn Agents A + B + C in parallel
2. Main does validation fix + sidebar restructure while agents work
3. Collect i18n key additions from all agents
4. Merge i18n keys into all 5 locale toolbox.json files
5. Run verification:
   ```bash
   cd ui/web && pnpm exec tsc --noEmit
   cd ui/web && pnpm build
   go build ./...
   go build -tags sqliteonly ./...
   ```
6. Deploy to server, commit + push

---

## Verification Checklist
- [ ] Storyboard validation: submit disabled when source empty for image/video scenes
- [ ] Sidebar: 3 items (Video Editor, Tools, Watermark) all navigate correctly
- [ ] Watermark: image tab works, video tab processes short clips <30s
- [ ] Watermark: before/after comparison slider works
- [ ] Video: canvas player shows storyboard preview
- [ ] Video: timeline drag-reorder scenes, undo/redo works
- [ ] Video: overload detection shows correct recommendation
- [ ] Video: server render fallback works
- [ ] Clouds: storage at bottom, icons thinner, file type badges visible
- [ ] Clouds: preview settings in modal
- [ ] All i18n: en, vi, zh, ko, ru complete
- [ ] TypeScript clean, Vite build clean, Go build clean
