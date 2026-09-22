import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";

/** One row of GET /v1/skills/market (bundled-skill catalog). */
export interface MarketSkill {
  slug: string;
  name: string;
  description: string;
  category: string;
  version?: string;
  requires?: string[];
  installed: boolean;
  installedVersion?: number;
  updateAvailable?: boolean;
}

interface MarketListResponse {
  skills: MarketSkill[];
  total: number;
  bundledDir?: string;
}

/** Result of POST /v1/skills/market/install — synchronous, no job layer. */
export interface MarketInstallResult {
  installed: string[];
  alreadyInstalled: string[];
  grantErrors?: string[];
  errors?: string[];
}

export const marketQueryKey = ["skills", "market"] as const;

/**
 * Skill market catalog + install/uninstall/update mutations. All endpoints
 * are synchronous JSON (local directory copies), so there is no job polling —
 * each mutation resolves when the gateway finished the operation.
 */
export function useSkillMarket() {
  const http = useHttp();
  const connected = useAuthStore((s) => s.connected);
  const queryClient = useQueryClient();

  const {
    data: skills = [],
    isFetching: loading,
    isError: error,
    refetch,
  } = useQuery({
    queryKey: marketQueryKey,
    queryFn: async () => {
      const res = await http.get<MarketListResponse>("/v1/skills/market");
      return res.skills ?? [];
    },
    staleTime: 60_000,
    enabled: connected,
  });

  const invalidate = useCallback(async () => {
    // Market flags + the installed list both change on every mutation.
    await queryClient.invalidateQueries({ queryKey: marketQueryKey });
    await queryClient.invalidateQueries({ queryKey: ["skills"] });
  }, [queryClient]);

  const install = useCallback(
    async (slugs: string[]) => {
      try {
        const res = await http.post<MarketInstallResult>("/v1/skills/market/install", { slugs });
        await invalidate();
        if (res.installed.length > 0) {
          toast.success(i18next.t("skills:market.installSuccess", { count: res.installed.length }));
        }
        if (res.alreadyInstalled.length > 0) {
          toast.info(i18next.t("skills:market.alreadyInstalled", { count: res.alreadyInstalled.length }));
        }
        if (res.errors && res.errors.length > 0) {
          toast.error(
            i18next.t("skills:market.installErrors", { count: res.errors.length }),
            res.errors.join("; "),
          );
        }
        return res;
      } catch (err) {
        toast.error(i18next.t("skills:market.installFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, invalidate],
  );

  const uninstall = useCallback(
    async (slug: string) => {
      try {
        const res = await http.delete<{ ok: string }>(
          `/v1/skills/market/installed/${encodeURIComponent(slug)}`,
        );
        await invalidate();
        toast.success(i18next.t("skills:market.uninstallSuccess", { name: slug }));
        return res;
      } catch (err) {
        toast.error(i18next.t("skills:market.uninstallFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, invalidate],
  );

  const update = useCallback(
    async (slug: string) => {
      try {
        const res = await http.post<{ ok: string }>(
          `/v1/skills/market/update/${encodeURIComponent(slug)}`,
          {},
        );
        await invalidate();
        toast.success(i18next.t("skills:market.updateSuccess", { name: slug }));
        return res;
      } catch (err) {
        toast.error(i18next.t("skills:market.updateFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, invalidate],
  );

  return { skills, loading, error, refetch, install, uninstall, update };
}
