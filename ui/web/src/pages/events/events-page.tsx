import { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { Radar, Trash2, Pause, Play, ArrowDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { useTeamEventStore } from "@/stores/use-team-event-store";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";
import { EventCard } from "./event-sections";
import type { TeamData } from "@/types/team";

const EVENT_CATEGORIES = [
  { label: "All", value: "all" },
  { label: "Delegation", value: "delegation" },
  { label: "Task", value: "team.task" },
  { label: "Message", value: "team.message" },
  { label: "Agent", value: "agent" },
  { label: "Team CRUD", value: "team.crud" },
  { label: "Agent Link", value: "agent_link" },
] as const;

export function EventsPage() {
  const allEvents = useTeamEventStore((s) => s.events);
  const paused = useTeamEventStore((s) => s.paused);
  const setPaused = useTeamEventStore((s) => s.setPaused);
  const clear = useTeamEventStore((s) => s.clear);

  const [categoryFilter, setCategoryFilter] = useState("all");
  const [teamFilter, setTeamFilter] = useState<string>("all");
  const [userFilter, setUserFilter] = useState<string>("all");
  const [chatFilter, setChatFilter] = useState<string>("all");
  const [isAtBottom, setIsAtBottom] = useState(true);
  const feedRef = useRef<HTMLDivElement>(null);

  // Team name resolution
  const ws = useWs();
  const [teamMap, setTeamMap] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    if (!ws.isConnected) return;
    ws.call<{ teams: TeamData[] }>(Methods.TEAMS_LIST)
      .then((res) => {
        const map = new Map<string, string>();
        for (const t of res.teams ?? []) {
          map.set(t.id, t.name);
        }
        setTeamMap(map);
      })
      .catch(() => {});
  }, [ws, ws.isConnected]);

  const resolveTeam = useCallback(
    (teamId: string | null): string => {
      if (!teamId) return "Global";
      return teamMap.get(teamId) ?? teamId.slice(0, 8);
    },
    [teamMap],
  );

  // Unique teams from events for filter dropdown
  const uniqueTeams = useMemo(() => {
    const ids = new Set<string>();
    for (const e of allEvents) {
      if (e.teamId) ids.add(e.teamId);
    }
    return Array.from(ids);
  }, [allEvents]);

  // Unique user IDs from events for filter dropdown
  const uniqueUsers = useMemo(() => {
    const ids = new Set<string>();
    for (const e of allEvents) {
      if (e.userId) ids.add(e.userId);
    }
    return Array.from(ids).sort();
  }, [allEvents]);

  // Unique chat IDs from events for filter dropdown
  const uniqueChats = useMemo(() => {
    const ids = new Set<string>();
    for (const e of allEvents) {
      if (e.chatId) ids.add(e.chatId);
    }
    return Array.from(ids).sort();
  }, [allEvents]);

  // Apply team filter
  const teamFilteredEvents = useMemo(() => {
    if (teamFilter === "all") return allEvents;
    return allEvents.filter((e) => e.teamId === teamFilter);
  }, [allEvents, teamFilter]);

  // Apply user filter
  const userFilteredEvents = useMemo(() => {
    if (userFilter === "all") return teamFilteredEvents;
    return teamFilteredEvents.filter((e) => e.userId === userFilter);
  }, [teamFilteredEvents, userFilter]);

  // Apply chat filter
  const chatFilteredEvents = useMemo(() => {
    if (chatFilter === "all") return userFilteredEvents;
    return userFilteredEvents.filter((e) => e.chatId === chatFilter);
  }, [userFilteredEvents, chatFilter]);

  // Apply category filter
  const filteredEvents = useMemo(() => {
    if (categoryFilter === "all") return chatFilteredEvents;
    if (categoryFilter === "team.crud") {
      return chatFilteredEvents.filter(
        (e) =>
          e.event === "team.created" ||
          e.event === "team.updated" ||
          e.event === "team.deleted" ||
          e.event.startsWith("team.member."),
      );
    }
    return userFilteredEvents.filter((e) => e.event.startsWith(categoryFilter));
  }, [chatFilteredEvents, categoryFilter]);

  // Auto-scroll to bottom when new events arrive
  useEffect(() => {
    if (isAtBottom && feedRef.current) {
      feedRef.current.scrollTop = feedRef.current.scrollHeight;
    }
  }, [filteredEvents.length, isAtBottom]);

  const handleScroll = () => {
    if (!feedRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = feedRef.current;
    setIsAtBottom(scrollHeight - scrollTop - clientHeight < 50);
  };

  const scrollToBottom = useCallback(() => {
    if (feedRef.current) {
      feedRef.current.scrollTop = feedRef.current.scrollHeight;
    }
    setIsAtBottom(true);
  }, []);

  return (
    <div className="p-4 sm:p-6">
      <PageHeader
        title="Realtime Events"
        description="Live WebSocket events across all teams and agents"
        actions={
          <>
            <Badge variant={paused ? "warning" : "success"} className="text-xs">
              {paused ? "Paused" : "Live"}
            </Badge>
            <span className="text-xs text-muted-foreground">
              {filteredEvents.length} event{filteredEvents.length !== 1 ? "s" : ""}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPaused(!paused)}
              className="h-8 gap-1.5"
            >
              {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
              {paused ? "Resume" : "Pause"}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={clear}
              className="h-8 gap-1.5"
            >
              <Trash2 className="h-3.5 w-3.5" /> Clear
            </Button>
          </>
        }
      />

      {/* Filters */}
      <div className="mt-4 flex flex-wrap items-center gap-3">
        {/* Category pills */}
        <div className="flex flex-wrap items-center gap-1.5">
          {EVENT_CATEGORIES.map((cat) => (
            <button
              key={cat.value}
              type="button"
              onClick={() => setCategoryFilter(cat.value)}
              className={`rounded-full px-2.5 py-0.5 text-xs transition-colors ${
                categoryFilter === cat.value
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {cat.label}
            </button>
          ))}
        </div>

        {/* Team filter */}
        {uniqueTeams.length > 0 && (
          <select
            value={teamFilter}
            onChange={(e) => setTeamFilter(e.target.value)}
            className="h-7 rounded-md border bg-background px-2 text-xs"
          >
            <option value="all">All teams</option>
            {uniqueTeams.map((id) => (
              <option key={id} value={id}>
                {resolveTeam(id)}
              </option>
            ))}
          </select>
        )}

        {/* User filter */}
        <select
          value={userFilter}
          onChange={(e) => setUserFilter(e.target.value)}
          className="h-7 rounded-md border bg-background px-2 text-xs"
        >
          <option value="all">All users</option>
          {uniqueUsers.map((uid) => (
            <option key={uid} value={uid}>
              {uid}
            </option>
          ))}
        </select>

        {/* Chat filter */}
        <select
          value={chatFilter}
          onChange={(e) => setChatFilter(e.target.value)}
          className="h-7 rounded-md border bg-background px-2 text-xs"
        >
          <option value="all">All chats</option>
          {uniqueChats.map((cid) => (
            <option key={cid} value={cid}>
              {cid}
            </option>
          ))}
        </select>
      </div>

      {/* Event feed */}
      <div className="mt-4 rounded-md border">
        {filteredEvents.length === 0 ? (
          <div className="px-4 py-12">
            <EmptyState
              icon={Radar}
              title="No events yet"
              description={
                paused
                  ? "Event capture is paused. Resume to see new events."
                  : "Waiting for real-time events... Events will appear as agents work."
              }
            />
          </div>
        ) : (
          <div className="relative">
            <div
              ref={feedRef}
              onScroll={handleScroll}
              className="max-h-[calc(100vh-280px)] min-h-[200px] space-y-2 overflow-y-auto p-3"
            >
              {filteredEvents.map((entry) => (
                <EventCard key={entry.id} entry={entry} resolveTeam={resolveTeam} />
              ))}
            </div>

            {/* Scroll-to-bottom FAB */}
            {!isAtBottom && (
              <button
                type="button"
                onClick={scrollToBottom}
                className="absolute bottom-4 right-4 flex h-8 w-8 items-center justify-center rounded-full border bg-background shadow-md transition-colors hover:bg-muted"
                title="Scroll to bottom"
              >
                <ArrowDown className="h-4 w-4" />
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
