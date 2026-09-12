---
name: mail-digest
description: Daily email digest for connected Gmail accounts — summarize the last 24h, group by sender, flag newsletter candidates for unsubscription (propose only, never auto-execute). Use when the user asks "check my mail", "daily mail digest", or schedules it via cron.
---

# Mail Digest

Tổng hợp mail 24h qua cho các tài khoản Gmail đã kết nối (qua cloud tools).

## Các bước

1. `cloud_accounts` — lấy danh sách tài khoản mail. Nếu rỗng, báo user kết
   nối trên trang Cloud (không đoán).
2. Với mỗi tài khoản:
   - `mail_search` với query `is:inbox newer_than:1d`, max 50.
   - Nhóm kết quả theo sender (domain/email trong `from`).
   - Với mỗi sender: đếm số mail, liệt kê tiêu đề chính (rút gọn).
   - Sender có ≥3 mail/quảng cáo: đánh dấu "ứng viên unsubscribe".
3. Viết báo cáo ngắn (theo ngôn ngữ user):
   - Tổng số mail, số sender.
   - Top 5 sender theo số mail — một dòng tóm tắt nội dung chính mỗi sender.
   - Danh sách "ứng viên unsubscribe" (sender + số mail + đã có
     List-Unsubscribe header — xác nhận qua `mail_read` nếu cần).
   - Mail quan trọng (không phải promo): liệt kê đầy đủ hơn.
4. Đề xuất hành động: "muốn anh/chị unsubscribe sender X không?" — CHỈ ĐỀ XUẤT.

## Quy tắc bắt buộc

- KHÔNG bao giờ tự gọi `mail_unsubscribe` với `execute=true` — luôn hỏi và
  chờ user xác nhận rõ ràng cho từng sender (dùng `ask_options`).
- KHÔNG gọi `mail_archive` hàng loạt nếu user chưa đồng ý danh sách cụ thể.
- Giới hạn: tối đa 50 mail/tài khoản/lần chạy; bỏ qua label
  CATEGORY_PROMOTIONS khi tổng hợp "quan trọng" nhưng vẫn đếm cho ứng viên
  unsubscribe.
- Nếu tool trả lỗi tài khoản (revoked/expired): báo user vào trang Cloud để
  kết nối lại, bỏ qua tài khoản đó và chạy tiếp tài khoản khác.
