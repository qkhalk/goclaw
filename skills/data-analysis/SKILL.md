---
name: data-analysis
description: "Analyze tabular data (CSV/Excel/JSON) with pandas: inspect and profile datasets, clean and transform columns, aggregate and pivot, compute statistics, and render charts to PNG. Use whenever the user uploads or references a data file and asks to analyze it, find trends or anomalies, summarize by group, compare periods, make a chart or report from data, merge/join datasets, or clean messy columns (dates, prices with currency symbols, diacritics). Triggers: phân tích dữ liệu, phân tích file, thống kê, biểu đồ, báo cáo từ dữ liệu, analyze data, dataset, CSV, Excel, pandas, pivot, chart. Do NOT use for scraping websites (use the scraping skill) or for single-value lookups a plain file read answers faster."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - dataset
  - question
outputs:
  - analysis_report
allowed-tools:
  - filesystem
  - shell
  - exec
quality-gates:
  - data_profiled
  - numbers_verified
  - report_generated
deps:
  - pip:pandas
  - pip:matplotlib
  - pip:openpyxl
---

# Data Analysis (pandas)

Turn CSV/Excel/JSON files into answers: profile, clean, aggregate, visualize,
report.

## Setup check (heavy — approval gate)

```bash
python3 -c "import pandas" 2>/dev/null || echo MISSING_PANDAS
```

pandas + matplotlib + openpyxl is a **~200 MB** install. If missing, do NOT
install silently — ask the user with `ask_options`, stating the size ("Cần cài
pandas + matplotlib (~200MB) để phân tích file. Cài chứ?"), and only install on
approval: `pip3 install pandas matplotlib openpyxl`.

## Workflow

1. **Profile first (never skip):** run the helper script before any analysis —
   column types, nulls, duplicates, ranges, sample rows:
   ```bash
   cd {baseDir}
   python3 scripts/profile.py data.csv
   python3 scripts/profile.py data.xlsx --sheet "Doanh thu" --rows 8
   ```
   Read the JSON carefully: dtypes tell you what cleaning is needed (numbers
   stored as text, dates as strings, nulls concentration).
2. **Clarify the question** if vague: "phân tích giúp anh" is not a question —
   propose 2–3 concrete analyses from the profile and let the user pick
   (use `ask_options` when it fits).
3. **Clean deliberately** in a short pandas script:
   - Vietnamese CSVs from Excel are often `utf-8-sig`; failed decode → try
     that encoding, then `cp1258`.
   - Money columns: strip currency symbols/thousand separators (`1.250.000₫`,
     `1,250,000 VND`) before `astype(float)`.
   - Dates: `pd.to_datetime(col, dayfirst=True, errors="coerce")` for
     dd/mm/yyyy; count the NaT afterward and say how many rows were lost.
   - Deduplicate with `df.duplicated()` from the profile, not blindly.
4. **Aggregate to answers:** `groupby`/`pivot_table`/`resample` for periods.
   Keep intermediate results as small named DataFrames; print the key numbers.
5. **Verify (mandatory):** cross-check at least two headline numbers a second
   way (e.g. sum vs len, groupby totals vs overall total). Wrong numbers in a
   confident report is the worst failure mode of this skill.
6. **Report:** lead with the answer to the user's question, then the supporting
   table (Markdown), then caveats (dropped rows, assumptions, data range).
   Numbers in prose must match the tables exactly.

## Charts

```python
import matplotlib
matplotlib.use("Agg")              # headless server — always set before pyplot
import matplotlib.pyplot as plt

fig, ax = plt.subplots(figsize=(8, 4.5))
df.groupby("month")["revenue"].sum().plot(kind="bar", ax=ax)
ax.set_title("Doanh thu theo tháng")
fig.tight_layout()
fig.savefig("chart_revenue.png", dpi=150)   # workspace path
```

- Save PNGs under the agent workspace and reference the file path in the reply.
- Vietnamese labels work with default fonts on most hosts; if labels render as
  boxes (□), install `fonts-dejavu` (small) or switch labels to English.
- Chart per message: 1–2 meaningful charts, not a gallery of everything.

## Large files

- Over ~200 MB or ~2M rows: don't load whole — use `pd.read_csv(...,
  chunksize=100_000)` and aggregate per chunk, or read only needed columns
  (`usecols=`). Say the file size and row count in the report.
- Excel >50 MB: prefer converting to CSV first (`in2csv`/openpyxl stream) —
  `.xlsx` parsing is memory-hungry.

## Output conventions

- Analysis scripts live in the workspace (`analysis/*.py`) so runs are
  reproducible; datasets stay where they are (never mutate the user's source
  file — write `*_processed.csv` instead).
- Rounding: state units (VND? nghìn tỷ?) and round consistently (2 decimals
  unless the domain says otherwise).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `No module named 'pandas'` | Approval gate above, then `pip3 install pandas matplotlib openpyxl` |
| `UnicodeDecodeError` | Retry `encoding="utf-8-sig"`, then `cp1258`; check file origin |
| Numbers parsed as text | Inspect raw values first (thousand separators? currency? spaces?); clean then convert |
| Dates all NaT | Wrong dayfirst/format — inspect samples, pass explicit `format=` |
| Chart labels are boxes | `apt install fonts-dejavu` or English labels |
| MemoryError | `usecols=`, `dtype=` on load, or chunked processing |
