---
name: document-skills
description: >-
  Router for the office-document suite (docx, pdf, pptx, xlsx, ocr): pick the right document skill
  per task, combine them into pipelines such as data to spreadsheet to chart to deck, and decide
  the output format with a decision matrix. Use when a task produces or consumes an office
  document. Keywords: document, docx, pdf, pptx, xlsx, excel, word, slides, ocr, chon dinh dang,
  tai lieu. Dùng khi cần tạo hoặc xử lý tài liệu văn phòng và chưa rõ dùng skill nào.
license: MIT
version: 1
---

# Document Skills

Route any office-document task to the right specialist skill and compose them into document pipelines.

## When to use
- The deliverable or the input is a .docx, .pdf, .pptx, or .xlsx file, or a scanned document.
- The user names a format ("make me a deck") or a goal ("turn this report into a spreadsheet").
- You are chaining data work into a formatted deliverable.

## When NOT to use
- Plain markdown output suffices — no office format was requested.
- Web content or images — use `web_browse` or `create_image` instead.

## Workflow
1. Identify the input format and the required output format using the matrix: **xlsx** for tabular data a user will filter or recalculate; **docx** for flowing text meant for editing and review; **pdf** for fixed final form that must render identically everywhere; **pptx** for a linear presentation to an audience; **ocr** for extracting text from images or scans first.
2. When unsure between two formats, ask one question with both options and trade-offs (see `ask`), or default to xlsx for data and docx for prose.
3. Load the matching specialist via `use_skill` (`xlsx`, `docx`, `pdf`, `pptx`, or `ocr`) and follow its workflow for creation or extraction.
4. For chains, finish each stage to a saved file before starting the next: data to deliverable = `databases` or `data-analysis` → `xlsx` with chart sheet → `pptx` summary slides; scan to editable = `ocr` → cleanup → `docx`; report to archive = `docx` → `pdf`; research brief = `research` → `docs` markdown draft → `docx` or `pdf` final.
5. Verify the output opens and renders: check page/sheet count and spot-check content against the source data.

## Output
The requested document file at the agreed path, plus a one-line note of the route taken (for example "ocr then docx").

## Routing
- The chosen specialist's own Routing section takes precedence for document-internal decisions.
- Source data lives in a database → `databases` first; numbers need interpretation → `data-analysis`.

## Guardrails
- Never fabricate document content: derive figures from real sources and mark placeholders clearly.
- Confirm format and filename/path with the user when the request says only "a report".
