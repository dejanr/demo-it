/**
 * Mind Map Extension
 *
 * Monitors architecture-relevant changes and prompts to review the mind map.
 */

import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { Type } from "@sinclair/typebox";
import * as fs from "node:fs";
import * as path from "node:path";

const MIND_MAP_FILE = "MIND_MAP.md";
const PROMPT_SCORE_THRESHOLD = 2;

const IGNORE_PATTERNS = [
  /(^|\/)MIND_MAP\.md$/,
  /(^|\/)flake\.lock$/,
  /(^|\/)pnpm-lock\.yaml$/,
  /(^|\/)package-lock\.json$/,
  /(^|\/)yarn\.lock$/,
  /(^|\/)node_modules\//,
  /(^|\/)\.devenv\//,
  /(^|\/)\.direnv\//,
  /(^|\/)\.expo\//,
  /\.gen\./,
];

const SIGNIFICANT_RULES: Array<{ pattern: RegExp; score: number }> = [
  // Repo-level architecture wiring
  { pattern: /^flake\.nix$/, score: 2 },
  { pattern: /(^|\/)devenv\.nix$/, score: 1.5 },

  // Infrastructure and backend architecture
  { pattern: /^infra\//, score: 2 },
  { pattern: /^backend\/(api|auth)\/(cmd|internal)\//, score: 2 },
  { pattern: /^database\/(lib|migrations)\//, score: 2 },

  // Frontend workspace architecture-level wiring
  {
    pattern: /^frontend\/(web|desktop|mobile)\/(flake\.nix|devenv\.nix)$/,
    score: 1.5,
  },
  { pattern: /^frontend\/(web|desktop|mobile)\/src\//, score: 1 },

  // Coordination docs that often reshape architecture boundaries
  { pattern: /(^|\/)AGENTS\.md$/, score: 1.5 },
];

interface TrackedChange {
  filePath: string;
  score: number;
}

interface ChangeTracker {
  significantChanges: TrackedChange[];
  lastPromptTime: number;
}

export default function (pi: ExtensionAPI) {
  const tracker: ChangeTracker = {
    significantChanges: [],
    lastPromptTime: 0,
  };

  function normalizePath(filePath: string): string {
    return filePath.replace(/\\/g, "/");
  }

  function calculateSignificance(filePath: string): number {
    const normalizedPath = normalizePath(filePath);

    if (IGNORE_PATTERNS.some((pattern) => pattern.test(normalizedPath))) {
      return 0;
    }

    return SIGNIFICANT_RULES.reduce((maxScore, rule) => {
      if (rule.pattern.test(normalizedPath)) {
        return Math.max(maxScore, rule.score);
      }
      return maxScore;
    }, 0);
  }

  function summarizeChanges(changes: TrackedChange[]) {
    const byPath = new Map<string, number>();
    for (const change of changes) {
      byPath.set(
        change.filePath,
        Math.max(byPath.get(change.filePath) ?? 0, change.score),
      );
    }

    const uniqueChanges = [...byPath.entries()]
      .sort((a, b) => b[1] - a[1])
      .map(([filePath, score]) => ({ filePath, score }));

    const totalScore = uniqueChanges.reduce(
      (sum, change) => sum + change.score,
      0,
    );

    return { uniqueChanges, totalScore };
  }

  async function getGitChangedPaths(): Promise<string[]> {
    const result = await pi.exec("git", ["status", "--porcelain"]);
    if (result.code !== 0) {
      return [];
    }

    return result.stdout
      .split("\n")
      .map((line) => line.trimEnd())
      .filter((line) => line.length > 3)
      .map((line) => {
        // Porcelain format: XY <path> or XY <old> -> <new>
        const rawPath = line.slice(3);
        const renamedTo = rawPath.includes(" -> ")
          ? (rawPath.split(" -> ").at(-1) ?? rawPath)
          : rawPath;
        return normalizePath(renamedTo);
      });
  }

  async function isMindMapChangedInGit(): Promise<boolean> {
    const result = await pi.exec("git", [
      "status",
      "--porcelain",
      "--",
      MIND_MAP_FILE,
    ]);
    return result.code === 0 && result.stdout.trim().length > 0;
  }

  function toTrackedChanges(filePaths: string[]): TrackedChange[] {
    return filePaths
      .map((filePath) => ({ filePath, score: calculateSignificance(filePath) }))
      .filter((change) => change.score > 0);
  }

  // Track write/edit operations for architecture-relevant files
  pi.on("tool_result", async (event, ctx) => {
    if (!ctx.hasUI) return;

    const { toolName, input } = event;

    if (toolName === "write" || toolName === "edit") {
      const filePath = (input as { path?: string }).path;
      if (!filePath) return;

      const score = calculateSignificance(filePath);
      if (score > 0) {
        tracker.significantChanges.push({
          filePath: normalizePath(filePath),
          score,
        });
      }
    }
  });

  // Pre-commit check: prompt before git commit tool runs
  pi.on("tool_call", async (event, ctx) => {
    if (!ctx.hasUI) return;
    if (event.toolName !== "git_commit_with_user_approval") return;

    const input = event.input as { files?: string[] };
    const requestedFiles = (input.files ?? []).map(normalizePath);

    const candidatePaths =
      requestedFiles.length > 0 ? requestedFiles : await getGitChangedPaths();
    const gitChanges = toTrackedChanges(candidatePaths);
    const inMemoryChanges =
      requestedFiles.length > 0 ? [] : tracker.significantChanges;

    const { uniqueChanges, totalScore } = summarizeChanges([
      ...inMemoryChanges,
      ...gitChanges,
    ]);

    if (totalScore < PROMPT_SCORE_THRESHOLD) {
      return;
    }

    const mindMapIncludedInCommit = requestedFiles.includes(MIND_MAP_FILE);
    const mindMapChanged = await isMindMapChangedInGit();

    if (mindMapIncludedInCommit || mindMapChanged) {
      return;
    }

    const changeList = uniqueChanges
      .slice(0, 5)
      .map((change) => `${change.filePath} (${change.score.toFixed(1)})`)
      .join("\n  - ");

    const moreCount =
      uniqueChanges.length > 5 ? `\n  (+${uniqueChanges.length - 5} more)` : "";

    const shouldPauseForReview = await ctx.ui.confirm(
      "Mind map review",
      `Architecture-relevant changes detected (score ${totalScore.toFixed(1)}):\n  - ${changeList}${moreCount}\n\nReview MIND_MAP.md before this commit?\n\nYes = stop commit now. No = continue commit.`,
    );

    if (shouldPauseForReview) {
      pi.sendUserMessage(
        "Please review whether MIND_MAP.md needs updates for these durable architecture changes before committing.",
        { deliverAs: "followUp" },
      );
      return {
        block: true,
        reason: "Blocked for mind map review before commit.",
      };
    }
  });

  // At end of agent turn, prompt only when threshold is reached
  pi.on("agent_end", async (_event, ctx) => {
    if (!ctx.hasUI) return;

    const changes = tracker.significantChanges;
    if (changes.length === 0) return;

    // Debounce: don't prompt more than once per 5 minutes
    const now = Date.now();
    if (now - tracker.lastPromptTime < 5 * 60 * 1000) {
      tracker.significantChanges = [];
      return;
    }

    const { uniqueChanges, totalScore } = summarizeChanges(changes);

    if (totalScore < PROMPT_SCORE_THRESHOLD) {
      tracker.significantChanges = [];
      return;
    }

    const mindMapPath = path.join(ctx.cwd, MIND_MAP_FILE);
    const mindMapExists = fs.existsSync(mindMapPath);

    const changeList = uniqueChanges
      .slice(0, 5)
      .map((change) => `${change.filePath} (${change.score.toFixed(1)})`)
      .join("\n  - ");

    const moreCount =
      uniqueChanges.length > 5 ? ` (+${uniqueChanges.length - 5} more)` : "";

    const message = mindMapExists
      ? `Architecture-relevant changes detected (score ${totalScore.toFixed(1)}):\n  - ${changeList}${moreCount}\n\nReview whether MIND_MAP.md needs updates for durable architecture/capability changes.`
      : `Architecture-relevant changes detected (score ${totalScore.toFixed(1)}):\n  - ${changeList}${moreCount}\n\nNo MIND_MAP.md found. Consider creating one.`;

    ctx.ui.notify(message, "info");
    tracker.significantChanges = [];
    tracker.lastPromptTime = now;
  });

  // Command to create/update mind map
  pi.registerCommand("mindmap", {
    description: "Create or update the project mind map",
    handler: async (args, ctx) => {
      const mindMapPath = path.join(ctx.cwd, MIND_MAP_FILE);
      const exists = fs.existsSync(mindMapPath);

      if (args === "status") {
        if (exists) {
          const content = fs.readFileSync(mindMapPath, "utf-8");
          const nodeCount = (content.match(/^\[/gm) || []).length;
          ctx.ui.notify(`MIND_MAP.md exists with ${nodeCount} nodes`, "info");
        } else {
          ctx.ui.notify("No MIND_MAP.md found", "warning");
        }
        return;
      }

      const action = exists ? "update" : "create";
      pi.sendUserMessage(
        `Please ${action} the MIND_MAP.md file. ` +
          (exists
            ? "Read the current mind map and update only nodes affected by durable architectural changes."
            : "Create a new mind map following the mind-map skill guidelines. Start with 5 overview nodes covering the main architecture."),
      );
    },
  });

  // Tool to add a node to the mind map
  pi.registerTool({
    name: "mindmap_add_node",
    label: "Add Mind Map Node",
    description:
      "Add a new node to MIND_MAP.md. Use this when documenting new concepts or features.",
    parameters: Type.Object({
      id: Type.Number({ description: "Node ID number" }),
      title: Type.String({ description: "Node title (concise)" }),
      description: Type.String({
        description: "Node description with [N] references to related nodes",
      }),
    }),

    async execute(_toolCallId, params, _onUpdate, ctx, _signal) {
      const { id, title, description } = params as {
        id: number;
        title: string;
        description: string;
      };

      const mindMapPath = path.join(ctx.cwd, MIND_MAP_FILE);
      const nodeLine = `[${id}] **${title}** - ${description}\n`;

      try {
        if (fs.existsSync(mindMapPath)) {
          const content = fs.readFileSync(mindMapPath, "utf-8");
          if (content.includes(`[${id}]`)) {
            return {
              content: [
                {
                  type: "text",
                  text: `Error: Node [${id}] already exists. Use edit to modify it.`,
                },
              ],
              isError: true,
            };
          }
          fs.appendFileSync(mindMapPath, nodeLine);
        } else {
          const header = `# ${path.basename(ctx.cwd)} Mind Map\n\n`;
          fs.writeFileSync(mindMapPath, header + nodeLine);
        }

        return {
          content: [{ type: "text", text: `Added node [${id}] **${title}**` }],
          details: { id, title },
        };
      } catch (error) {
        return {
          content: [
            {
              type: "text",
              text: `Failed to add node: ${(error as Error).message}`,
            },
          ],
          isError: true,
        };
      }
    },
  });
}
