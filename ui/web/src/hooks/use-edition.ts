import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";

/** Public edition info (GET /v1/edition — no auth required). */
export interface EditionInfo {
  name: string;
  cloud_accounts_enabled?: boolean;
}

export function useEdition() {
  const http = useHttp();
  return useQuery({
    queryKey: ["edition"],
    staleTime: 5 * 60_000,
    queryFn: () => http.get<EditionInfo>("/v1/edition"),
  });
}
