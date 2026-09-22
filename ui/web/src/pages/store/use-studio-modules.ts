import { useMemo } from "react";
import {
  useBuiltinTools,
  type BuiltinToolData,
} from "@/pages/builtin-tools/hooks/use-builtin-tools";
import { STUDIO_CATEGORY, STUDIO_MODULES, type StudioModuleMeta } from "./studio-modules";

export interface StudioModule {
  meta: StudioModuleMeta;
  /** Installed = tenant override when set, else the seeded global default. */
  installed: boolean;
  /** Undefined when the server predates the studio seed — module stays installed. */
  def?: BuiltinToolData;
}

/** Studio modules resolved against the builtin tools list (shared react-query cache). */
export function useStudioModules(): { modules: StudioModule[]; loading: boolean } {
  const { tools, loading } = useBuiltinTools();
  const modules = useMemo(
    () =>
      STUDIO_MODULES.map((meta) => {
        const def = tools.find((t) => t.name === meta.name && t.category === STUDIO_CATEGORY);
        return { meta, installed: def ? def.tenant_enabled ?? def.enabled : true, def };
      }).sort((a, b) => a.meta.order - b.meta.order),
    [tools],
  );
  return { modules, loading };
}
