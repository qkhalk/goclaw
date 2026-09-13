#!/usr/bin/env python3
"""profile.py — first-look profile of a CSV/Excel dataset (JSON to stdout).

Example:
  python3 scripts/profile.py sales.csv
  python3 scripts/profile.py sales.xlsx --sheet "Sheet2" --rows 30
"""
from __future__ import annotations

import argparse
import json
import sys


def die(msg: str, code: int = 2) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(code)


def main() -> None:
    parser = argparse.ArgumentParser(description="Dataset profile (pandas)")
    parser.add_argument("file", help="CSV or Excel file")
    parser.add_argument("--sheet", help="Excel sheet name (default: first)")
    parser.add_argument("--rows", type=int, default=5, help="sample rows to include (default 5)")
    parser.add_argument("--sep", default=None, help="CSV separator (default: pandas auto-sniff)")
    parser.add_argument("--encoding", default=None, help="e.g. utf-8, utf-8-sig, cp1258")
    args = parser.parse_args()

    try:
        import pandas as pd
    except ImportError:
        die(
            "pandas is not installed (~200 MB with matplotlib for charts). "
            "Ask the user for approval, then: pip3 install pandas matplotlib openpyxl"
        )

    lower = args.file.lower()
    try:
        if lower.endswith((".xlsx", ".xls")):
            df = pd.read_excel(args.file, sheet_name=args.sheet or 0)
        else:
            # sep=None + engine="python" lets pandas auto-sniff the delimiter
            df = pd.read_csv(args.file, sep=args.sep, engine="python", encoding=args.encoding)
    except UnicodeDecodeError:
        if args.encoding:
            die(f"cannot decode with {args.encoding}")
        # Excel-exported Vietnamese CSVs are usually utf-8-sig
        df = pd.read_csv(args.file, sep=args.sep, engine="python", encoding="utf-8-sig")

    def col_summary(s):
        info = {
            "dtype": str(s.dtype),
            "nulls": int(s.isna().sum()),
            "unique": int(s.nunique(dropna=True)),
        }
        if s.dtype.kind in "if" or str(s.dtype).startswith("Float") or str(s.dtype).startswith("Int"):
            d = s.describe()
            info.update({"min": float(d["min"]), "max": float(d["max"]), "mean": round(float(d["mean"]), 4)})
        else:
            top = s.mode(dropna=True)
            info["sample"] = str(top.iloc[0])[:80] if len(top) else None
        return info

    profile = {
        "file": args.file,
        "rows": int(len(df)),
        "columns": list(map(str, df.columns)),
        "dtypes": {str(k): str(v) for k, v in df.dtypes.items()},
        "summary": {str(c): col_summary(df[c]) for c in df.columns},
        "null_ratio": round(float(df.isna().mean().mean()), 4),
        "duplicated_rows": int(df.duplicated().sum()),
        "sample": json.loads(df.head(max(0, args.rows)).to_json(orient="records", force_ascii=False)),
    }
    print(json.dumps(profile, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
