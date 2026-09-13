---
name: ocr
description: "Extract text from images and scanned PDFs with Tesseract OCR, Vietnamese-first (vie+eng). Use whenever the user sends a photo/screenshot/scan and wants the text read, transcribed, searchable, or summarized; when asked to digitize receipts, invoices, forms, ID-style documents they own, book pages, or handwritten-ish print; when a PDF has no text layer (scanned) and text extraction is needed. Triggers: OCR, đọc chữ trong ảnh, trích xuất chữ, quét văn bản, scan to text, ảnh chụp tài liệu, hoá đơn, receipt, screenshot text, PDF scan. Do NOT use for images that are primarily objects/scenes without text, or for handwriting tesseract cannot read (report the limitation instead)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - image_or_pdf
outputs:
  - extracted_text
allowed-tools:
  - filesystem
  - shell
  - exec
quality-gates:
  - text_extracted
  - accuracy_checked
deps:
  - system:tesseract
  - pip:pytesseract
  - pip:Pillow
---

# OCR (Vietnamese-first)

Read text out of images and scanned PDFs with Tesseract. Vietnamese is the
primary language; English rides along by default.

## Setup check

```bash
tesseract --version >/dev/null 2>&1 || echo MISSING_TESSERACT
python3 -c "import pytesseract, PIL" 2>/dev/null || pip3 install pytesseract Pillow
tesseract --list-langs 2>/dev/null | grep -q vie || echo MISSING_VIE
```

If `MISSING_TESSERACT`: `apt install tesseract-ocr` (~30 MB with data) — small
enough to install directly, mention it in the report. If `MISSING_VIE`:
`apt install tesseract-ocr-vie`. Missing **pip** packages: install directly
(light). These are small installs — no approval gate needed; anything heavy
(>100 MB) always requires asking first.

## Usage — helper script

```bash
cd {baseDir}

# Image -> text printed to stdout
python3 scripts/ocr.py photo.jpg

# Vietnamese-only, single text block (better for signs/cover pages)
python3 scripts/ocr.py photo.jpg --lang vie --psm 6

# Low-quality / blurry / skewed photo: enable preprocessing
python3 scripts/ocr.py blurry.jpg --preprocess --out clean.txt

# Multi-page: several images in order
python3 scripts/ocr.py page1.jpg page2.jpg page3.jpg --out book.txt

# Scanned PDF (no text layer) — renders pages at 300 DPI via poppler
python3 scripts/ocr.py invoice.pdf --out invoice.txt
```

`--psm` cheat sheet: `3` auto (default) · `6` single block of text · `7` single
line · `11` sparse text (forms, labels) · `13` raw line.

## Workflow

1. **Look before OCR.** If the image is attached in chat, you can often read it
   directly (vision). Use OCR when the user wants a faithful transcription, a
   file output, searchable text, or the image is only available as a file.
2. **Extract** with the script (default `--lang vie+eng --psm 3`).
3. **Quality check (mandatory):** re-read the output. If it is mostly garbage
   (symbol soup, <50% plausible words), retry once with `--preprocess` and
   `--psm 6`. If still garbage, say so honestly — do not invent text.
4. **Deliver:** faithful text for transcription requests; a structured summary
   (items/amounts/dates as a table) for receipts and invoices; Markdown with
   `-----` page markers for books/multi-page scans.
5. **Save** outputs under the agent workspace (`ocr/<name>.txt`), never system
   paths.

## Field tips for Vietnamese documents

- Vietnamese diacritics suffer at small sizes: prefer `--preprocess` for
  photos taken with a phone; keep `vie` first in `--lang` so diacritics win.
- Receipts/invoices: OCR numbers confidently wrong 5–10% of the time — mark
  uncertain amounts with `(?)` instead of presenting them as fact.
- Tables: OCR reads columns line-by-line and scrambles them — extract with
  `--psm 6`, then rebuild the table structure yourself from the text layout.
- Scanned PDF with a text layer (copyable text in a viewer): skip OCR, extract
  the text directly instead.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `tesseract is not installed` / not found | `apt install tesseract-ocr` |
| `Failed loading language 'vie'` | `apt install tesseract-ocr-vie` |
| `pdftoppm not found` | `apt install poppler-utils` (small) |
| Garbage output on phone photos | `--preprocess` + `--psm 6`; ensure the image is right-side up |
| Diacritics mangled | Keep `vie` first in `--lang`; use `--preprocess`; higher DPI for PDFs (`--dpi 400`) |
| Wrong text on plain background screenshots | Try `--psm 11` (sparse) or `--psm 7` per line |
