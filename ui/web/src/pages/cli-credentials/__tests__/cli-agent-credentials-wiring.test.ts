import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function source(path: string): string {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("CLI agent credential UI wiring", () => {
  it("exposes Agent Credentials as a distinct table action before advanced user overrides", () => {
    const table = source("src/pages/cli-credentials/cli-credentials-table.tsx");

    expect(table).toContain("onAgentCreds");
    expect(table).toContain("agentCredentials.title");
    expect(table.indexOf("onAgentCreds(item)")).toBeLessThan(table.indexOf("onUserCreds(item)"));
  });

  it("mounts the Agent Credentials dialog from the panel", () => {
    const panel = source("src/pages/cli-credentials/cli-credentials-panel.tsx");

    expect(panel).toContain("cli-agent-credentials-dialog");
    expect(panel).toContain("CLIAgentCredentialsDialog");
    expect(panel).toContain("agentCredsTarget");
  });
});
