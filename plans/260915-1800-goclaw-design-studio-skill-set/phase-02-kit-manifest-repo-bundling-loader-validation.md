---
phase: 2
title: "Kit manifest + repo bundling + loader validation"
status: pending
priority: P1
effort: "0.5d"
dependencies: [phase-01]
---

# Phase 2: Kit manifest + repo bundling + loader validation

## Overview

Đóng gói 12 skill thành kit `design-studio` có manifest, thêm Go test chặn regression (parse + BM25 hit), và chạy full validation trong repo.

## Requirements

- Functional: `skills/design-studio/kit.yaml` nhận diện được qua `internal/skills/kit_manager.go`; test tự động chạy trong `go test ./internal/skills/`
- Non-functional: test không cần DB/server (chạy trên thư mục + loader thuần)

## Architecture

`kit.yaml` theo schema `{name, version, description, skills: [slugs]}` — checksum do kit manager tự tính, không hand-author (pattern `skills/go-claw-engineer/kit.yaml` deployed trên server).

Test file mới `internal/skills/design_studio_test.go`:
- Parse: load 12 thư mục qua loader, không error/warn, metadata đúng (name=slug, description non-empty)
- Budget: SKILL.md body ≤10KB mỗi skill pinned
- BM25: index 12 skill, assert query "storyboard" / "bảng màu" / "type scale" trả đúng slug top-3 (dùng `internal/skills/search.go` trực tiếp)

## Related Code Files

- Create: `skills/design-studio/kit.yaml`
- Create: `internal/skills/design_studio_test.go`
- Modify: không
- Delete: không

## Implementation Steps

1. Viết `kit.yaml` (12 slugs đúng thứ tự pipeline)
2. Viết `design_studio_test.go` theo 3 nhóm assert trên (table-driven, pattern của `sanitize_test.go`)
3. `go test ./internal/skills/ -run DesignStudio -v` xanh
4. `go build ./... && go vet ./...` sạch

## Success Criteria

- [ ] `go test ./internal/skills/ -run DesignStudio` pass trong CI (không cần integration tags)
- [ ] BM25 asserts: 3/3 query hit đúng slug top-3
- [ ] Repo bundle structure khớp server layout (12 dir + kit dir)

## Risk Assessment

- Test phụ thuộc đường dẫn tương đối bundle: locate repo root như test hiện có của `internal/skills` — nếu CI chạy từ thư mục khác, fix bằng embed fixture hoặc skip theo build tag. Tín hiệu: CI red với "skills dir not found".

