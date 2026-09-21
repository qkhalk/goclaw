import { useEffect } from "react";
import { useUiStore, type Theme } from "@/stores/use-ui-store";

/// Applies a Theme to the document root. Exported for surfaces that force a
/// temporary theme (e.g. the video studio) and need to restore the user's
/// CURRENT theme on unmount — a mount-time snapshot goes stale if the user
/// (or the OS, in system mode) changes theme while they are on the page.
export function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.classList.remove("light", "dark");

  if (theme === "system") {
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    root.classList.add(systemDark ? "dark" : "light");
  } else {
    root.classList.add(theme);
  }
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const theme = useUiStore((s) => s.theme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  // Listen for system theme changes when in "system" mode
  useEffect(() => {
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => applyTheme("system");
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [theme]);

  return <>{children}</>;
}
