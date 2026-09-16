import { useTranslation } from "react-i18next";
import { Circle, CircleCheck, ListTodo, LoaderCircle } from "lucide-react";
import type { ToolStreamEntry } from "@/types/chat";

interface PlanStep {
  text: string;
  status: "pending" | "in_progress" | "completed";
}

interface PlanView {
  steps: PlanStep[];
}

const isPlanStatus = (v: unknown): v is PlanStep["status"] =>
  v === "pending" || v === "in_progress" || v === "completed";

/**
 * Extract the checklist state from a plan tool entry. The tool's ForLLM
 * result is the canonical JSON state followed by a human-readable render, so
 * the JSON prefix is authoritative; arguments (action=set items) are the
 * fallback while the call is still streaming. Returns null when neither
 * yields a usable checklist — the generic tool card renders instead.
 */
export function parsePlanEntry(entry: ToolStreamEntry): PlanView | null {
  const fromResult = parsePlanResult(entry.result);
  if (fromResult) return fromResult;
  return parsePlanArguments(entry.arguments);
}

function parsePlanResult(result: string | undefined): PlanView | null {
  if (!result) return null;
  const jsonLine = result.split("\n", 1)[0]?.trim();
  if (!jsonLine || !jsonLine.startsWith("{")) return null;
  try {
    const parsed: unknown = JSON.parse(jsonLine);
    return normalizeSteps((parsed as { steps?: unknown }).steps);
  } catch {
    return null;
  }
}

function parsePlanArguments(args: Record<string, unknown> | undefined): PlanView | null {
  if (!args || args.action !== "set" || !Array.isArray(args.items)) return null;
  return normalizeSteps(
    args.items.map((text) => ({ text, status: "pending" })),
  );
}

function normalizeSteps(raw: unknown): PlanView | null {
  if (!Array.isArray(raw)) return null;
  const steps: PlanStep[] = [];
  for (const item of raw) {
    if (typeof item === "string") {
      // arguments fallback carries bare strings
      if (item.trim()) steps.push({ text: item, status: "pending" });
      continue;
    }
    if (
      item &&
      typeof item === "object" &&
      typeof (item as PlanStep).text === "string" &&
      isPlanStatus((item as PlanStep).status)
    ) {
      steps.push({ text: (item as PlanStep).text, status: (item as PlanStep).status });
    }
  }
  return steps.length > 0 ? { steps } : null;
}

/**
 * Read-only checklist card for the plan tool: the agent mutates the plan via
 * plan tool calls and the user watches progress here. The latest call in a
 * message shows the newest state; earlier calls keep their historical state.
 */
export function PlanCard({ view }: { view: PlanView }) {
  const { t } = useTranslation("common");
  const done = view.steps.filter((s) => s.status === "completed").length;
  const total = view.steps.length;
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;

  return (
    <div className="rounded-lg border border-border bg-background p-2.5">
      <div className="flex items-center gap-2">
        <ListTodo className="h-4 w-4 shrink-0 text-blue-500" />
        <span className="text-sm font-medium">{t("planCard.title")}</span>
        <span className="ml-auto text-xs tabular-nums text-muted-foreground">
          {t("planCard.progress", { done, total })}
        </span>
      </div>

      <div
        className="mt-2 h-1 overflow-hidden rounded-full bg-muted"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
      >
        <div
          className="h-full rounded-full bg-blue-500 transition-all duration-500"
          style={{ width: `${pct}%` }}
        />
      </div>

      <ol className="mt-2 space-y-1">
        {view.steps.map((step, i) => (
          <li key={i} className="flex items-start gap-2 text-sm">
            <StepIcon status={step.status} />
            <span
              className={`min-w-0 flex-1 break-words ${
                step.status === "completed"
                  ? "text-muted-foreground line-through"
                  : step.status === "in_progress"
                    ? "font-medium"
                    : ""
              }`}
            >
              {step.text}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function StepIcon({ status }: { status: PlanStep["status"] }) {
  if (status === "completed") {
    return <CircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-600" />;
  }
  if (status === "in_progress") {
    return <LoaderCircle className="mt-0.5 h-4 w-4 shrink-0 animate-spin text-blue-500" />;
  }
  return <Circle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />;
}
