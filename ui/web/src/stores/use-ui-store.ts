import { create } from "zustand";
import { persist } from "zustand/middleware";
import i18n from "@/i18n";
import { type Language } from "@/lib/constants";

export type Theme = "light" | "dark" | "system";

// Draggable column widths (px). Clamped in the setters so a stray persisted
// value or an aggressive drag can never break the chat axis layout.
export const NAV_SIDEBAR_WIDTH = { min: 224, max: 320, default: 256 } as const;
export const CHAT_SIDEBAR_WIDTH = { min: 220, max: 440, default: 288 } as const;
export const CHAT_PANE_WIDTH = { min: 320, max: 720, default: 384 } as const;

function clampWidth(w: number, range: { min: number; max: number }): number {
  return Math.max(range.min, Math.min(range.max, Math.round(w)));
}

interface UiState {
  theme: Theme;
  language: Language;
  timezone: string; // IANA timezone or "auto"
  sidebarCollapsed: boolean;
  mobileSidebarOpen: boolean;
  pageSize: number; // global pagination page size preference
  navSidebarWidth: number; // global nav sidebar (expanded), px
  chatSidebarWidth: number; // chat session-list column, px
  chatPaneWidth: number; // right tabbed side pane, px

  setTheme: (theme: Theme) => void;
  setLanguage: (language: Language) => void;
  setTimezone: (tz: string) => void;
  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setMobileSidebarOpen: (open: boolean) => void;
  setPageSize: (size: number) => void;
  setNavSidebarWidth: (w: number) => void;
  setChatSidebarWidth: (w: number) => void;
  setChatPaneWidth: (w: number) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set, get) => ({
      theme: "dark" as Theme,
      language: (i18n.language as Language) ?? "en",
      timezone: "auto",
      sidebarCollapsed: false,
      mobileSidebarOpen: false,
      pageSize: 20,
      navSidebarWidth: NAV_SIDEBAR_WIDTH.default,
      chatSidebarWidth: CHAT_SIDEBAR_WIDTH.default,
      chatPaneWidth: CHAT_PANE_WIDTH.default,

      setTheme: (theme) => {
        set({ theme });
      },

      setLanguage: (language) => {
        i18n.changeLanguage(language);
        set({ language });
      },

      setTimezone: (tz) => {
        set({ timezone: tz });
      },

      toggleSidebar: () => {
        set({ sidebarCollapsed: !get().sidebarCollapsed });
      },

      setSidebarCollapsed: (collapsed) => {
        set({ sidebarCollapsed: collapsed });
      },

      setMobileSidebarOpen: (open) => set({ mobileSidebarOpen: open }),

      setPageSize: (size) => set({ pageSize: size }),

      setNavSidebarWidth: (w) => set({ navSidebarWidth: clampWidth(w, NAV_SIDEBAR_WIDTH) }),
      setChatSidebarWidth: (w) => set({ chatSidebarWidth: clampWidth(w, CHAT_SIDEBAR_WIDTH) }),
      setChatPaneWidth: (w) => set({ chatPaneWidth: clampWidth(w, CHAT_PANE_WIDTH) }),
    }),
    {
      name: "goclaw:ui", // localStorage key
      partialize: (state) => ({
        // Persist user preferences — not transient UI state
        theme: state.theme,
        language: state.language,
        timezone: state.timezone,
        sidebarCollapsed: state.sidebarCollapsed,
        pageSize: state.pageSize,
        navSidebarWidth: state.navSidebarWidth,
        chatSidebarWidth: state.chatSidebarWidth,
        chatPaneWidth: state.chatPaneWidth,
      }),
    }
  )
);
