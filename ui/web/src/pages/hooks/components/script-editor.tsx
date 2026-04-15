import { useMemo } from "react";
import Editor from "react-simple-code-editor";
import Prism from "prismjs";
import "prismjs/components/prism-javascript";
import { useTranslation } from "react-i18next";

interface ScriptEditorProps {
  value: string;
  onChange: (v: string) => void;
  // Server-returned compile error. goja prints "(anonymous): Line N:M message"
  // (see goja parser/error.go — "%s: Line %d:%d %s"). We extract the location
  // with the regex below; when the shape doesn't match we surface the raw
  // string unchanged.
  error?: string;
  readOnly?: boolean;
}

// parseLineCol extracts {line, col} from a goja compile-error string. Format
// is "... Line N:M ...". Any other shape returns null so the caller can fall
// back to rendering the raw message.
export function parseLineCol(err?: string): { line: number; col: number } | null {
  if (!err) return null;
  const m = err.match(/Line (\d+):(\d+)/);
  if (!m || !m[1] || !m[2]) return null;
  return { line: parseInt(m[1], 10), col: parseInt(m[2], 10) };
}

export function ScriptEditor({ value, onChange, error, readOnly }: ScriptEditorProps) {
  const { t } = useTranslation("hooks");
  const loc = useMemo(() => parseLineCol(error), [error]);

  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium">{t("form.scriptSource")}</label>
      <div className="rounded-lg border bg-muted/30 overflow-hidden">
        <Editor
          value={value}
          onValueChange={readOnly ? () => {} : onChange}
          highlight={(code) => {
            const lang = Prism.languages.javascript;
            return lang ? Prism.highlight(code, lang, "javascript") : code;
          }}
          padding={12}
          textareaClassName="outline-none"
          readOnly={readOnly}
          className="text-base md:text-sm font-mono min-h-[240px] leading-relaxed"
          style={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace" }}
        />
      </div>
      {error ? (
        <p className="text-xs text-destructive">
          {loc
            ? t("form.compileError", { line: loc.line, col: loc.col })
            : error}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">{t("form.scriptSchemaHelp")}</p>
      )}
    </div>
  );
}
