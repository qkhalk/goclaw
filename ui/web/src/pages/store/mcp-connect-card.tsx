import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Copy, TerminalSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";

/**
 * Copy-paste snippets for attaching external MCP clients (Claude Code,
 * OpenCode, Cursor/Cline) to the gateway's tool bridge (/mcp/bridge).
 * The gateway token is deliberately never prefilled — the admin pastes it
 * from their secret store so it cannot leak into screenshots or logs.
 */
export function McpConnectCard() {
  const { t } = useTranslation("tools");
  const [copied, setCopied] = useState<string | null>(null);

  const origin = useMemo(() => {
    if (typeof window === "undefined") return "http://localhost:18790";
    return window.location.origin;
  }, []);
  const bridgeUrl = `${origin}/mcp/bridge`;
  const TOKEN = "<GATEWAY_TOKEN>";

  const snippets = useMemo(
    () => [
      {
        id: "claude",
        label: "Claude Code",
        code: `claude mcp add --transport http goclaw ${bridgeUrl} \\\n  --header "Authorization: Bearer ${TOKEN}"`,
      },
      {
        id: "opencode",
        label: "OpenCode",
        code: JSON.stringify(
          {
            mcp: {
              goclaw: {
                type: "remote",
                url: bridgeUrl,
                headers: { Authorization: `Bearer ${TOKEN}` },
              },
            },
          },
          null,
          2,
        ),
      },
      {
        id: "cursor",
        label: "Cursor / Cline",
        code: JSON.stringify(
          {
            mcpServers: {
              goclaw: {
                url: bridgeUrl,
                headers: { Authorization: `Bearer ${TOKEN}` },
              },
            },
          },
          null,
          2,
        ),
      },
    ],
    [bridgeUrl],
  );

  async function copy(code: string, id: string) {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(id);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      toast.error(t("store.mcp.copy_failed"));
    }
  }

  return (
    <section className="mt-8 rounded-lg border p-4">
      <div className="flex items-center gap-2">
        <TerminalSquare className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-semibold">{t("store.mcp.title")}</h2>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{t("store.mcp.desc")}</p>
      <Tabs defaultValue="claude" className="mt-3">
        <TabsList className="grid w-full max-w-md grid-cols-3">
          {snippets.map((s) => (
            <TabsTrigger key={s.id} value={s.id} className="text-base md:text-sm">
              {s.label}
            </TabsTrigger>
          ))}
        </TabsList>
        {snippets.map((s) => (
          <TabsContent key={s.id} value={s.id} className="mt-2">
            <div className="relative">
              <pre className="overflow-x-auto rounded-md bg-muted p-3 pr-12 text-xs leading-relaxed">
                <code>{s.code}</code>
              </pre>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t("store.mcp.copy")}
                onClick={() => void copy(s.code, s.id)}
                className={cn(
                  "absolute right-1 top-1 h-8 w-8 min-h-8 min-w-8 sm:h-9 sm:w-9 sm:min-h-9 sm:min-w-9",
                  "text-muted-foreground hover:text-foreground",
                )}
              >
                {copied === s.id ? <Check className="h-4 w-4 text-primary" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </TabsContent>
        ))}
      </Tabs>
      <p className="mt-2 text-xs text-muted-foreground">{t("store.mcp.token_hint")}</p>
    </section>
  );
}
