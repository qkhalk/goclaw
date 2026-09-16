# Báo cáo: Video Designer Agent Column (plan 260915-1243) + Browser Panel fixes

Ngày: 2026-09-15. Deploy: `goclaw` v4.11.0-designer @ 192.168.1.103 (gateway 18790 + videoworker 18791).

## A. Browser panel fixes (plans/browser-panel-improvements.md)

- **1.9 sandbox live-mode**: `ui/web/src/components/chat/browser-panel.tsx` — `allow-same-origin` chỉ được thêm khi `finalUrl` origin ≠ origin dashboard (chống thoát sandbox). Static relay giữ `allow-same-origin`.
- **1.10 modeSource**: `use-browser-panel.ts` — thêm `modeSource: "auto" | "user"`; toggle của user pin vĩnh viễn (không bị auto-fallback ghi đè), auto-fallback chỉ chạy khi `modeSource === "auto"`, ngược lại hiện `thinStaticNote`.
- **2.8 layout floor**: `chat-page.tsx` main column `min-w-[min(360px,100vw)]`; `chat-sidebar.tsx` `shrink-0` + width override qua style — kéo resize không còn nghiền nát layout.
- **1.1/1.3**: `internal/tools/web_fetch.go` — UA Chrome/129 Windows, `Accept-Language: en,*;q=0.5`, HTTP ≥400 trả error kèm 256 bytes body thật.
- **1.7**: đã có sẵn (mobile zoom).
- i18n `browserPanel.thinStaticNote` thêm đủ 5 locale (en/vi/zh/ko/ru), không em-dash.

## B. Video designer column (5 phase)

### Phase 1 — backend
- `internal/video/designer_agent.go`: `EnsureDesignerAgent()` — idempotent theo `agent_key=video-designer`, master tenant; `ToolsConfig = {"profile":"minimal","allow":["skill_search","use_skill","session_status"]}`; IDENTITY.md persona design-only (20-60s, 3-8 cảnh, luôn kết thúc bằng fence ```storyboard).
- 2 skill design gốc (không mang skill cá nhân của anh sang): `bundled-skills/video-storyboard-design/` (cấu trúc hook/body/close, caption ≤8 từ, hợp đồng JSON v1) + `bundled-skills/video-color-motion/` (palette #RRGGBB, Ken Burns 1.0→1.12, transition).
- `grantDesignerSkills()`: chuyển 2 skill thành `visibility=internal, is_system=false` + grant riêng cho video-designer → không agent nào khác thấy.
- Wire tại `cmd/gateway.go` (goroutine sau pricing sync, chỉ chạy khi `videoStack != nil`).
- **Fix trong lúc code**: PG `GetByKey` trả `(nil, err)` khi không thấy → ensure phải coi not-found là path create (theo pattern agents_create.go).
- Test: `internal/video/designer_agent_test.go` (fake stores: create/idempotency/không ghi đè agent đã sửa/tolerate skill thiếu) + `internal/tools/designer_policy_test.go` (FilterTools trên registry đầy đủ → đúng 3 tool). `go build` 2 edition + vet xanh.
- Dọn file test mồ côi `internal/tools/shell_timeout_test.go` (tham chiếu symbol không tồn tại, chặn build test).

### Phase 2/3 — UI
- `use-ui-store.ts`: `VIDEO_DESIGNER_WIDTH` (320/560/384) + `videoDesignerOpen`, persist.
- `chat-input.tsx`: prop `storageKey` (designer dùng `goclaw.composer-override:video-designer`, hết chia sẻ localStorage với chat chính).
- `use-designer-chat.ts`: session key `agent:video-designer:ws:direct:<convId>` persist `goclaw.video-designer-conv`; wrap useChatMessages/useChatSend; newChat xoay convId.
- `designer-column.tsx`: rail resize (ResizeHandle, floor editor 480px) ≥1024px, Sheet bottom `<1024px`; header Palette + status dot + new-chat + close; empty state 3 chips; attach-current storyboard; status "đang chốt storyboard...".
- `lib/parse-storyboard-blocks.ts`: extract fence ```storyboard (fence chưa đóng khi stream = tự bỏ qua) + parse shape lỏng theo contract.
- `storyboard-card.tsx`: chip tỉ lệ + n cảnh + tổng giây (tabular-nums), scene strip màu thật, Apply/Xem JSON, badge "Đã áp dụng", card lỗi đỏ khi JSON sai.
- `video-tool-page.tsx`: tách `applyStoryboardToEditor()` dùng chung JSON mode + designer; layout flex + sticky rail; nút toggle ở PageHeader.
- i18n `video.designer.*` 20 key × 5 locale.

### Phase 5 — deploy + verify live (Edge thật, 7 bước)
1. ✅ Mở /tools/video → toggle "Trình thiết kế video" → rail mở, chips + composer riêng.
2. ✅ Gửi "Tạo intro 9 giây, 3 cảnh gradient tím sang cam..." → agent **kích hoạt đúng 2 skill design** (skill: video-storyboard-design, video-color-motion — Đã kích hoạt → Hoàn thành) → StoryboardCard 9:16 · 3 cảnh · 9s, strip màu tím→magenta→cam.
3. ✅ "Áp dụng vào timeline" → timeline 3 cảnh · 9.0s, canvas render nền tím #6A0DAD, scene editor đúng caption "CHÀO MỪNG!", transition Chuyển mờ; badge "Đã áp dụng"; Hoàn tác bật.
4. ✅ "Đính kèm storyboard hiện tại" + "giảm còn 6 giây" → message mang prefix "[Storyboard hiện tại để tham khảo]" + JSON đầy đủ → card mới 3 cảnh · 6s; Apply → timeline 6.0s; **2 card độc lập**, undo/redo quay lại đúng 9s/6s.
5. ✅ Negativa: "chạy lệnh shell giúp tôi" → reply TEXT từ chối, **0 tool call**: "Mình không có quyền chạy lệnh shell... mình chỉ thiết kế storyboard thôi."
6. ✅ F5 reload → cột vẫn mở, history replay đầy đủ (kèm 2 card vẫn Apply được). Chat chính song song: kiến trúc session key khác nhau (đã tách storageKey composer);未 chạy song song 2 cửa sổ trong verify này.
7. ⚠️ Mobile 375px: chưa verify live (bước F12 emulation dính nhầm window Edge cá nhân của anh nên dừng để không đụng window riêng); code dùng Sheet chuẩn `ui/sheet.tsx` + `max-sm:inset-0` + safe-area theo AGENTS.md — cần kiểm tra nhanh trên điện thoại.

## Lỗi bắt được khi verify live (đã sửa + deploy lại)
1. `crypto.randomUUID is not a function` — HTTP thường (LAN) không có secure context → dùng `uniqueId()` có fallback sẵn trong `lib/utils.ts`. Đây là lý do trang lỗi lúc đầu.
2. Agent tạo với provider mặc định config (anthropic) không có credential trên server → UPDATE designer sang provider `1k` / `oc/mimo-v2.5-free` (cùng provider agent fox-spirit đang chạy tốt). Lưu ý: nếu config.json `agents.defaults` vẫn trỏ provider không có credential thì agent mới tạo sau này cũng sẽ dính — cân nhắc sửa defaults trong config.

## Trạng thái DB sau deploy (đối chiếu plan)
- agents: `video-designer | active | {"allow": ["skill_search","use_skill","session_status"]}`
- skills: 2 skill `internal`, `is_system=f`, granted cho video-designer.
- journal: `video: designer agent ensured agent_key=video-designer` (mỗi boot, idempotent, không tạo trùng).

## Còn treo
- **2 commit local chưa push** trên `feat/client-browser-panel`: `6256d6a9b`, `de1996554` (+ các thay đổi session này chưa commit). Chờ anh duyệt push.
- `go test ./internal/tools/` full package treo ≥600s ở máy local (goroutine delegate_tool chờ network — có sẵn, không phải do thay đổi; test CI vẫn xanh).
- `internal/agent` media_test fail sẵn trên Windows (package không đụng thay đổi này).
- Mobile 375px verify (mục 7) — cần 5 phút kiểm tra trên điện thoại.
