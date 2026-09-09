---
title: "Phase 3: Docker packages and verification"
status: todo
---

# Phase 3: Docker packages and verification

## Overview

Docker full variant (ENABLE_FULL_SKILLS=true) cài sẵn công cụ cho 4 skill test mới (đã verify tên package Alpine qua pkgs.alpinelinux.org trước khi ghi Dockerfile), cập nhật docs runtime, và chạy bước verify toàn cục cuối cùng.

## Requirements

- [x] Verify tên package Alpine (apk) cho: nmap, nikto, wrk, hey, iperf3, hping3, sqlmap, testssl.sh — chỉ thêm vào Dockerfile những package chắc chắn tồn tại trong Alpine community/main repo; phần còn lại rely vào install-deps runtime.
- [x] Dockerfile full-skills block (`Dockerfile:75-96`) thêm `apk add --no-cache <packages>` cho các tool đã verify.
- [x] `docs/14-skills-runtime.md`: cập nhật bảng package full-variant nếu thêm package.
- [x] Verify toàn cục: `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, test các package bị đụng (channels/telegram, config, skills), confirm không regression (các failure Windows pre-existing ở internal/agent + internal/tools đã được chứng minh ở session trước — không phải do thay đổi này).
- [x] Audit-verify pass (AGENTS.md rule 17): spawn Explore agent kiểm tra claims chính của implementation (wiring callers đủ, HTML escape, chunk, whitelist, deps format).
- [x] Commit chi tiết theo conventional commits + Surface parity, push branch, PR `--repo qkhalk/goclaw`, merge vào dev.

## Implementation Steps

1. WebSearch/verify Alpine package names.
2. Dockerfile edit + docs.
3. Full build/test matrix local.
4. Audit verify → fix findings.
5. Commit + push + PR + merge.

## Todo

- [x] Alpine package names verified
- [x] Dockerfile + docs updated
- [x] Build/test matrix xanh
- [x] Audit verify pass
- [x] PR merged

## Success Criteria

- [x] Docker build args không đổi hành vi mặc định (ENABLE_FULL_SKILLS=false giữ nguyên image như cũ).
- [x] CI Linux xanh trên PR.

## Risk Assessment

- **Tên apk sai làm gãy docker build** → verify từng tên trước; chỉ thêm package chắc chắn. Tín hiệu vỡ: CI docker build fail → bỏ package lỗi ra, ghi vào install-deps path.
- **image size phình** (nmap+nikto+sqlmap ~200MB) → chỉ nằm ở full variant (đã là variant nặng); base/latest không ảnh hưởng.
