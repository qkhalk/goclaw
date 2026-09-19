/** Shared recharts tooltip styling bound to the app theme tokens. Recharts
 * renders tooltips with inline styles — without these, the label falls back
 * to a fixed light-theme gray that is unreadable in dark mode (the "Ngày:"
 * label bug on the usage charts). Item colors still come from each series'
 * stroke, so only the chrome is themed here. */
export const chartTooltipProps = {
  contentStyle: {
    backgroundColor: "var(--card)",
    border: "1px solid var(--border)",
    borderRadius: 8,
  },
  labelStyle: { color: "var(--foreground)", fontWeight: 500 },
  cursor: { stroke: "var(--muted-foreground)", strokeDasharray: "3 3" },
} as const;
