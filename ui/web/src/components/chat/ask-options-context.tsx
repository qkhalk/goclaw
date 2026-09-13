import { createContext, useContext, type ReactNode } from "react";

/**
 * Bridges ask_options question cards to the chat send path. Provided by the
 * chat page; absent on read-only surfaces (session detail) where cards render
 * non-interactive. The answer format mirrors the Telegram inject exactly
 * ("[Answering your question] <question> → <answer>") so both channels share
 * the same turn-taking contract.
 */
export interface AskOptionsContextValue {
  /** Send the user's picked/typed answer as the next user message. */
  answer: (question: string, answer: string) => void;
  /** True once a user message answering this exact question exists. */
  isAnswered: (question: string) => boolean;
}

export const AskOptionsContext = createContext<AskOptionsContextValue | null>(null);

export function useAskOptions(): AskOptionsContextValue | null {
  return useContext(AskOptionsContext);
}

/** The exact user-turn prefix the Telegram channel injects on button press. */
export function askAnswerText(question: string, answer: string): string {
  return `[Answering your question] ${question.trim()} → ${answer}`;
}

interface AskOptionsProviderProps {
  value: AskOptionsContextValue | null;
  children: ReactNode;
}

/** Convenience wrapper so the chat page can pass a possibly-null value. */
export function AskOptionsProvider({ value, children }: AskOptionsProviderProps) {
  return <AskOptionsContext.Provider value={value}>{children}</AskOptionsContext.Provider>;
}
