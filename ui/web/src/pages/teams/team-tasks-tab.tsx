import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { RefreshCw } from "lucide-react";
import type { TeamTaskData } from "@/types/team";
import { TaskList } from "./task-sections";

interface TeamTasksTabProps {
  teamId: string;
  getTeamTasks: (teamId: string) => Promise<{ tasks: TeamTaskData[]; count: number }>;
}

export function TeamTasksTab({ teamId, getTeamTasks }: TeamTasksTabProps) {
  const [tasks, setTasks] = useState<TeamTaskData[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getTeamTasks(teamId);
      setTasks(res.tasks ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [teamId, getTeamTasks]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={load} disabled={loading} className="gap-1.5">
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>
      <TaskList tasks={tasks} loading={loading} />
    </div>
  );
}
