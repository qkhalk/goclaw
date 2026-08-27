#!/usr/bin/env node
// Compute the next release tag and build release notes that record the exact
// fork delta versus upstream (nextlevelbuilder/goclaw).
//
// Outputs (GITHUB_OUTPUT):
//   released   "true" | "false"
//   version    e.g. "3.17.0" (plain) or "3.16.0-fork.1" (fork)
//   tag        e.g. "v3.17.0" (plain) or "v3.16.0-fork.1" (fork)
//   notes_path path to the generated release-notes.md
//
// Env:
//   FORK_BASE        base semver, default "3.17.0"
//   PRERELEASE_ID    prerelease id, default "fork"
//   VERSION_OVERRIDE if set, tag is pinned to this — for re-cuts / manual fixes.
//   TAG_MODE         "plain" (default) auto-increments patch (v3.17.0 → v3.17.1);
//                    "fork" appends -fork.N.
import { execFileSync } from "node:child_process";
import { writeFileSync, appendFileSync } from "node:fs";

const base = process.env.FORK_BASE || "3.16.0";
const prerelease = process.env.PRERELEASE_ID || "fork";
const override = process.env.VERSION_OVERRIDE || "";
const plainMode = (process.env.TAG_MODE || "fork") === "plain";
function git(args, opts) {
  return execFileSync("git", args, { encoding: "utf8", ...opts }).trim();
}

function setOutput(name, value) {
  const output = process.env.GITHUB_OUTPUT;
  if (output) {
    appendFileSync(output, `${name}=${value}\n`);
    return;
  }
  console.log(`${name}=${value}`);
}

function parseForkVersion(value) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)-fork\.(\d+)$/.exec(value);
  if (!m) return null;
  return {
    raw: value,
    major: Number(m[1]),
    minor: Number(m[2]),
    patch: Number(m[3]),
    n: Number(m[4]),
  };
}

function versionText(major, minor, patch) {
  return `${major}.${minor}.${patch}`;
}

function writeNoRelease(reason) {
  setOutput("released", "false");
  setOutput("version", "");
  setOutput("tag", "");
  console.log(reason);
}

// Latest upstream release tag (plain vX.Y.Z, no -beta/-rc/-fork) that is an
// ancestor of HEAD. Tags are compared numerically (semver), not lexically,
// so v3.13.2 beats v3.9.2. Never use an upstream prerelease as the delta base.
function upstreamAnchor(head) {
  const allTags = git(["tag", "--list", "v[0-9]*"]).split(/\r?\n/).filter(Boolean);
  const plain = allTags
    .filter((t) => /^v[0-9]+\.[0-9]+\.[0-9]+$/.test(t))
    .map((t) => {
      const m = /^v(\d+)\.(\d+)\.(\d+)$/.exec(t);
      return { raw: t, parts: [Number(m[1]), Number(m[2]), Number(m[3])] };
    })
    .sort((a, b) => {
      for (let i = 0; i < 3; i++) {
        if (a.parts[i] !== b.parts[i]) return a.parts[i] - b.parts[i];
      }
      return 0;
    })
    .reverse(); // newest first
  for (const tag of plain) {
    try {
      const commit = git(["rev-list", "-n", "1", tag.raw]);
      git(["merge-base", "--is-ancestor", commit, head]); // throws if not ancestor
      return tag.raw; // newest ancestor — the delta base
    } catch {
      /* not an ancestor of HEAD — keep searching */
    }
  }
  return null;
}

// Commit list HEAD..anchor as conventional-commit subjects grouped by kind.
function forkDelta(messages) {
  const groups = { feat: [], fix: [], perf: [], docs: [], chore: [], other: [] };
  for (const line of messages) {
    const header = line.split(/\r?\n/, 1)[0] || "";
    if (/^feat(?:\([^)]+\))?:/.test(header)) groups.feat.push(header);
    else if (/^fix(?:\([^)]+\))?:/.test(header)) groups.fix.push(header);
    else if (/^perf(?:\([^)]+\))?:/.test(header)) groups.perf.push(header);
    else if (/^docs(?:\([^)]+\))?:/.test(header)) groups.docs.push(header);
    else if (/^chore(?:\([^)]+\))?:/.test(header)) groups.chore.push(header);
    else if (header) groups.other.push(header);
  }
  return groups;
}

const head = process.env.GITHUB_SHA || git(["rev-parse", "HEAD"]);

// Fetch upstream tags so the fork delta base (newest plain upstream release
// merged into HEAD) resolves even on a fresh CI checkout. Local upstream tags
// exist too (the fork merges upstream manually), so a fetch failure is non-fatal.
try {
  git(["fetch", "--tags", "https://github.com/nextlevelbuilder/goclaw.git"], { stdio: "pipe" });
} catch {
  // Non-fatal: fall back to local upstream tags as ancestors.
}

// Compute the tag.
let tag;
if (plainMode) {
  // Plain mode: clean semver tag with no suffix (v3.17.0). Auto-increments
  // patch if the tag already exists (v3.17.0 → v3.17.1 → v3.17.2).
  let [major, minor, patch] = base.split(".").map(Number);
  if (![major, minor, patch].every(Number.isFinite)) {
    writeNoRelease(`FORK_BASE '${base}' is not valid semver for plain mode.`);
    process.exit(0);
  }
  // Auto-bump until we find an unused tag
  for (let tries = 0; tries < 100; tries++) {
    const candidate = `v${versionText(major, minor, patch)}`;
    if (!git(["tag", "--list", candidate])) {
      tag = candidate;
      break;
    }
    patch++;
  }
  if (!tag) {
    writeNoRelease(`Could not find unused tag after 100 increments from v${base}.`);
    process.exit(0);
  }
} else if (override) {
  const parsed = parseForkVersion(override);
  if (!parsed) {
    writeNoRelease(`VERSION_OVERRIDE '${override}' is not a valid fork tag (vX.Y.Z-fork.N).`);
    process.exit(0);
  }
  tag = "v" + override;
} else {
  const [major, minor, patch] = base.split(".").map(Number);
  const forkTags = git(["tag", "--list", `v${base}-fork.*`])
    .split(/\r?\n/)
    .filter(Boolean)
    .map(parseForkVersion)
    .filter(Boolean)
    .sort((a, b) => a.n - b.n);
  const last = forkTags[forkTags.length - 1];
  const n = last ? last.n + 1 : 1;
  tag = `v${versionText(major, minor, patch)}-${prerelease}.${n}`;
}

if (git(["tag", "--list", tag])) {
  if (override) {
    // Re-cut: an explicit VERSION_OVERRIDE is a deliberate rebuild of an
    // existing tag, so allow it (the workflow force-pushes the tag).
    console.log(`Re-cutting existing tag ${tag} (VERSION_OVERRIDE=${override}).`);
  } else {
    writeNoRelease(`Tag ${tag} already exists; delete it or set VERSION_OVERRIDE (re-cut).`);
    process.exit(0);
  }
}

// Fork-delta commit list relative to the upstream anchor.
const anchor = upstreamAnchor(head);
const anchorLabel = anchor || "upstream-anchor";
const subjectList = [];
let deltaMessage;
if (anchor) {
  const log = git(["log", "--format=%B%x1e", `${anchor}..HEAD`]);
  const messages = log.split("\x1e").map((m) => m.trim()).filter(Boolean);
  subjectList.push(...messages.map((m) => m.split(/\r?\n/, 1)[0]));
  deltaMessage = `upstream \`${anchor}\` → this release`;
} else {
  subjectList.push(...git(["log", "--format=%s", "-300"]).split(/\r?\n/).filter(Boolean));
  deltaMessage = "no upstream release tag found locally — listing recent commits";
}
const groups = forkDelta(subjectList);

const lines = [
  `## ${tag}`,
  "",
  `Fork release \`${tag}\` của **qkhalk/goclaw** — fork delta so với upstream (${deltaMessage}).`,
  "",
  "### Fork delta so với upstream",
  "",
  ...(groups.feat.length ? [`**Features**`, "", ...groups.feat.map((c) => `- ${c}`), ""] : []),
  ...(groups.fix.length ? [`**Fixes**`, "", ...groups.fix.map((c) => `- ${c}`), ""] : []),
  ...(groups.perf.length ? [`**Performance**`, "", ...groups.perf.map((c) => `- ${c}`), ""] : []),
  ...(groups.docs.length ? [`**Docs**`, "", ...groups.docs.map((c) => `- ${c}`), ""] : []),
  ...(groups.chore.length ? [`**Chore**`, "", ...groups.chore.map((c) => `- ${c}`), ""] : []),
  ...(groups.other.length ? [`**Other commits**`, "", ...groups.other.map((c) => `- ${c}`), ""] : []),
  "",
  "### Docker images",
  "",
  `- \`ghcr.io/qkhalk/goclaw:${tag}\``,
  `- \`ghcr.io/qkhalk/goclaw:${tag}-full\``,
  `- \`ghcr.io/qkhalk/goclaw:fork\` (alias)`,
  "",
  "Toàn bộ fork features (reliability layer + AgentKit phases) — xem mục **Fork Features** trong README.",
  "",
];

const notesPath = "release-notes.md";
writeFileSync(notesPath, lines.join("\n"));
setOutput("released", "true");
setOutput("version", tag.replace(/^v/, ""));
setOutput("tag", tag);
setOutput("notes_path", notesPath);
setOutput("anchor", anchorLabel);
console.log(`Next fork release: ${tag} (delta from ${anchorLabel})`);