import { useState, useEffect, useRef } from "react";
import { Search, UserPlus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { useContactSearch } from "../../hooks/use-contact-search";
import type { ChannelContact } from "@/types/contact";

/** Format a contact into a human-readable snippet for insertion into context files. */
function formatContactSnippet(c: ChannelContact): string {
  const parts: string[] = [];
  if (c.display_name) parts.push(c.display_name);
  if (c.username) parts.push(`@${c.username}`);
  parts.push(`${c.channel_type}:${c.sender_id}`);
  return `- ${parts.join(" — ")}`;
}

interface ContactInsertSearchProps {
  onInsert: (text: string) => void;
}

export function ContactInsertSearch({ onInsert }: ContactInsertSearchProps) {
  const { t } = useTranslation("agents");
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const { contacts } = useContactSearch(search);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const handleSelect = (c: ChannelContact) => {
    onInsert(formatContactSnippet(c));
    setSearch("");
    setOpen(false);
  };

  return (
    <div ref={containerRef} className="relative">
      <div className="relative">
        <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        <input
          value={search}
          onChange={(e) => { setSearch(e.target.value); setOpen(true); }}
          onFocus={() => search.length >= 2 && setOpen(true)}
          placeholder={t("files.insertContact")}
          className="h-8 w-full max-w-sm rounded-md border bg-transparent pl-7 pr-2 text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
        />
      </div>
      {open && search.length >= 2 && contacts.length > 0 && (
        <div className="absolute left-0 z-50 mt-1 max-h-48 w-full max-w-sm overflow-y-auto rounded-md border bg-popover p-1 shadow-md">
          {contacts.map((c) => (
            <button
              key={c.id}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => handleSelect(c)}
              className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-accent hover:text-accent-foreground"
            >
              <UserPlus className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <div className="truncate font-medium">
                  {c.display_name || c.sender_id}
                </div>
                <div className="flex items-center gap-1 text-[10px] text-muted-foreground">
                  {c.username && <span>@{c.username}</span>}
                  <Badge variant="outline" className="text-[9px] px-1 py-0">{c.channel_type}</Badge>
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
      {open && search.length >= 2 && contacts.length === 0 && (
        <div className="absolute left-0 z-50 mt-1 w-full max-w-sm rounded-md border bg-popover p-3 text-center text-xs text-muted-foreground shadow-md">
          {t("instances.noContactsFound")}
        </div>
      )}
    </div>
  );
}
