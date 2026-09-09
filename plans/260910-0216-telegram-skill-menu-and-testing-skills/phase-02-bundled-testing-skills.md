---
title: "Phase 2: Bundled testing skills"
status: todo
---

# Phase 2: Bundled testing skills

## Overview

Thêm 4 skill bundled mới trong `skills/` phục vụ kiểm thử sản phẩm trước khi lên production: audit bảo mật web (burp-like), load test HTTP Layer 7, stress test Layer 4 (TCP/UDP), audit TLS/SSL. Tất cả đều có **authorization gate** cứng ở đầu skill — chỉ chạy against hạ tầng mình sở hữu hoặc có xác nhận bằng văn bản.

## Requirements

- [ ] `skills/security-audit/SKILL.md` — đánh giá bảo mật có thẩm quyền: recon (nmap), TLS, security headers, nikto, dir discovery, SQLi/XSS scoped (sqlmap `--batch --level=1`), auth/session analysis, báo cáo findings theo severity + remediation.
- [ ] `skills/loadtest/SKILL.md` — capacity test L7: chọn tool (wrk/hey/hey→ab fallback), profiles (smoke/baseline/ramp/stress/soak), SLO definition, tìm "knee" (điểm gãy RPS/latency/error-rate), correlate server metrics (CPU, DB pool, FD), abort thresholds, pre-production gate report.
- [ ] `skills/netstress/SKILL.md` — resilience test L4: iperf3 TCP/UDP throughput + pps, connection-rate (hping3 rate-limited, fallback nping từ nmap), connection exhaustion ceiling, conntrack limits; chỉ chạy trong cửa sổ test đã lên lịch trên hạ tầng mình.
- [ ] `skills/ssl-audit/SKILL.md` — cert expiry/SAN/chain (openssl s_client), protocol versions (từ chối SSLv3/TLS1.0/1.1), cipher strength, HSTS, OCSP stapling; bảng kết quả + remediation. Fallback openssl thuần khi thiếu testssl.sh.
- [ ] Frontmatter mỗi skill theo mẫu `skills/cook/SKILL.md`: `name`, `description` (mô tả rõ khi nào dùng — đây là thứ BM25 search + `/skills` hiển thị), `license: Proprietary. Part of GoClaw bundled skills.`, `version: 1`, `inputs`, `outputs`, `allowed-tools`, `quality-gates`, và **`deps:`** (block list `system:...` / `pip:...`) — dep_scanner treats explicit deps as authoritative (`internal/skills/dep_manifest.go:141`, `applyManifestOverride`).
- [ ] Body: mọi skill có section "Authorization gate" đầu tiên: yêu cầu target thuộc sở hữu/người yêu cầu test có quyền; từ chối target bên ngoài scope; ghi nhận scope trước khi chạy; rate limit + stop conditions.
- [ ] `docs/15-core-skills-system.md`: thêm 4 skill vào bundled list.

## Architecture

Skills là prompt-injection thuần (LLM đọc SKILL.md qua `read_file` sau `use_skill`), không có engine riêng. Files nằm trong `skills/<slug>/SKILL.md`; seeder (`internal/skills/seeder.go:55-100`) tự đọc, hash, `UpsertSystemSkill`, copy sang managed dir khi gateway khởi động; `CheckDepsAsync` (`seeder.go:299`) đánh dấu missing deps (tool chưa cài) mà không chặn startup; admin bấm install-deps (`/v1/skills/install-deps` → `installManagedDeps`) cài qua apt/apk/pip tùy OS.

Không đụng code Go trong phase này (trừ docs). `allowed-tools` khai báo đúng nhóm tool thật mà skill cần (shell/exec/filesystem/web fetch).

Deps khai báo (giữ tối thiểu, các tool còn lại ghi hướng dẫn cài trong body):
- security-audit: `system:nmap`, `system:nikto`, `pip:sqlmap`, `system:testssl.sh`, `system:curl`
- loadtest: `system:wrk`, `system:hey`, `system:curl`
- netstress: `system:iperf3`, `system:hping3`, `system:curl`
- ssl-audit: `system:testssl.sh`, `system:openssl`

## Related Code Files

- Create: `skills/security-audit/SKILL.md`
- Create: `skills/loadtest/SKILL.md`
- Create: `skills/netstress/SKILL.md`
- Create: `skills/ssl-audit/SKILL.md`
- Modify: `docs/15-core-skills-system.md` (bundled list)

## Implementation Steps

1. Viết 4 SKILL.md (mỗi file ~150–300 dòng, tiếng Anh — bootstrap templates English-only theo AGENTS.md).
2. **Update `internal/skills/bundled_smoke_test.go`** (audit finding): `TestBundledSkills_NoRegression` hiện assert mọi bundled skill `FromManifest == false && Explicit empty` — 4 skill mới khai báo `deps:` sẽ fail. Sửa test: thêm map expected-manifest-deps cho 4 slug mới (assert đúng dep list khai báo), giữ assertion chặt cho các skill cũ. Đồng thời thêm 4 slug vào `expected` map của `TestBundledSkills_ExpectedCoreSkillSlugs` (hiện chỉ check frontmatter name/description cho {docx, goclaw, pdf, pptx, skill-creator, workspace-organizing, xlsx}).
3. Chạy `go test ./internal/skills/ -run "TestBundled|TestSeeder"` — bundled smoke + frontmatter parse phải sạch.

## Todo

- [ ] 4 SKILL.md với authorization gate + methodology + report format
- [ ] bundled_smoke_test.go update (expected deps + expected slugs)
- [ ] docs/15 bundled list update
- [ ] internal/skills tests pass (bundled smoke)

## Success Criteria

- [ ] Frontmatter parse sạch (name/description không rỗng, list fields đúng định dạng block-list).
- [ ] Mô tả đủ tốt cho BM25: chứa từ khóa "security", "load test", "stress", "TLS"…
- [ ] Authorization gate hiện diện ở đầu cả 4 skill; không có hướng dẫn tấn công target bên thứ ba (chỉ own-authorized infra).

## Risk Assessment

- **Tên package hệ thống khác nhau apt vs apk** (ví dụ testssl.sh) → install-deps report missing trên OS không có; body skill luôn có fallback thủ công (openssl/curl). Tín hiệu vỡ: install-deps lỗi resolve → thêm alias vào `aptSystemPackageAliases` (`system_package_installer.go:13`) — chỉ làm nếu thật sự cần.
- **Skill quá dài gây nhiễu prompt**: `BuildPinnedSummary` inline tối đa 10KB/skill — giữ mỗi SKILL.md gọn dưới ngưỡng này.
