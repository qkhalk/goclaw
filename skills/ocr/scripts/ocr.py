#!/usr/bin/env python3
"""ocr.py — Tesseract OCR helper with Vietnamese support and light preprocessing.

Examples:
  python3 scripts/ocr.py scan.jpg                     # vie+eng text -> stdout
  python3 scripts/ocr.py scan.jpg --lang vie --psm 6 --out result.txt
  python3 scripts/ocr.py page1.jpg page2.jpg --out book.txt
  python3 scripts/ocr.py doc.pdf --out doc.txt        # PDF via pdftoppm (poppler)
  python3 scripts/ocr.py blurry.jpg --preprocess      # grayscale + upscale + threshold
"""
from __future__ import annotations

import argparse
import os
import subprocess
import sys
import tempfile

SUPPORTED_IMAGE_EXT = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".bmp", ".webp", ".pnm", ".gif"}


def die(msg: str, code: int = 2) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(code)


def need(module: str, hint: str) -> None:
    try:
        __import__(module)
    except ImportError:
        die(f"missing python package {module!r}. Install with: {hint}")


def preprocess(path: str, tmpdir: str) -> str:
    """Grayscale + 2x upscale + Otsu-ish threshold for low-quality scans."""
    from PIL import Image, ImageFilter

    img = Image.open(path)
    if img.mode != "L":
        img = img.convert("L")
    w, h = img.size
    if max(w, h) < 2400:
        img = img.resize((w * 2, h * 2), Image.LANCZOS)
    img = img.filter(ImageFilter.MedianFilter(3))
    hist = img.histogram()
    total = sum(hist)
    best_t, best_var = 128, -1.0
    wsum = 0.0
    for t in range(256):
        wsum += t * hist[t]
    sum_b = 0.0
    w_b = 0
    for t in range(256):
        w_b += hist[t]
        if w_b == 0:
            continue
        w_f = total - w_b
        if w_f == 0:
            break
        sum_b += t * hist[t]
        m_b = sum_b / w_b
        m_f = (wsum - sum_b) / w_f
        var = w_b * w_f * (m_b - m_f) ** 2
        if var > best_var:
            best_var, best_t = var, t
    img = img.point(lambda p: 255 if p > best_t else 0)
    out = os.path.join(tmpdir, "pre_" + os.path.basename(path).rsplit(".", 1)[0] + ".png")
    img.save(out)
    return out


def images_from_pdf(path: str, tmpdir: str, dpi: int) -> list[str]:
    try:
        subprocess.run(
            ["pdftoppm", "-r", str(dpi), "-png", path, os.path.join(tmpdir, "pdfpage")],
            check=True,
        )
    except FileNotFoundError:
        die("pdftoppm not found (poppler-utils). Install: apt install poppler-utils — or convert PDF pages to images yourself")
    except subprocess.CalledProcessError as exc:
        die(f"pdftoppm failed on {path}: {exc}")
    pages = sorted(
        os.path.join(tmpdir, f) for f in os.listdir(tmpdir) if f.startswith("pdfpage") and f.endswith(".png")
    )
    if not pages:
        die(f"no pages rendered from {path}")
    return pages


def run_tesseract(img_path: str, lang: str, psm: int) -> str:
    try:
        import pytesseract
    except ImportError:
        die("pytesseract not installed. Run: pip3 install pytesseract  (also needs system tesseract)")
    try:
        return pytesseract.image_to_string(img_path, lang=lang, config=f"--psm {psm}")
    except pytesseract.TesseractError as exc:
        if "Failed loading language" in str(exc) or "is not loaded" in str(exc):
            die(
                f"tesseract language data {lang.split('+')[0]!r} missing. Install with:\n"
                f"  apt install tesseract-ocr-vie   # Vietnamese\n"
                f"  apt install tesseract-ocr-eng   # English"
            )
        die(f"tesseract failed on {img_path}: {exc}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Tesseract OCR (Vietnamese-first)")
    parser.add_argument("inputs", nargs="+", help="image files or a PDF")
    parser.add_argument("--lang", default="vie+eng", help="tesseract languages (default vie+eng)")
    parser.add_argument("--psm", type=int, default=3, help="page segmentation mode (3=auto, 6=block, 7=line, 11=sparse)")
    parser.add_argument("--dpi", type=int, default=300, help="render DPI for PDFs (default 300)")
    parser.add_argument("--preprocess", action="store_true", help="grayscale + upscale + threshold before OCR")
    parser.add_argument("--out", help="write text here (else print to stdout)")
    args = parser.parse_args()

    need("PIL", "pip3 install Pillow")
    if not args.preprocess:
        need("pytesseract", "pip3 install pytesseract")

    texts: list[str] = []
    with tempfile.TemporaryDirectory() as tmpdir:
        for path in args.inputs:
            if not os.path.isfile(path):
                die(f"not a file: {path}")
            ext = os.path.splitext(path)[1].lower()
            if ext == ".pdf":
                pages = images_from_pdf(path, tmpdir, args.dpi)
            elif ext in SUPPORTED_IMAGE_EXT:
                pages = [path]
            else:
                die(f"unsupported file type {ext!r}: {path}")

            for i, page in enumerate(pages):
                src = preprocess(page, tmpdir) if args.preprocess else page
                if i > 0 or len(texts):
                    texts.append("\n\n-----\n\n")
                texts.append(run_tesseract(src, args.lang, args.psm).strip())

    text = "".join(texts)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            fh.write(text)
        words = len(text.split())
        print(f"ocr ok: {len(args.inputs)} file(s) -> {args.out} ({words} words)")
    else:
        print(text)


if __name__ == "__main__":
    main()
