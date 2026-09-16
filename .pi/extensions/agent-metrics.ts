import { randomUUID } from "node:crypto";
import { basename } from "node:path";
import type {
  ExtensionAPI,
  ExtensionCommandContext,
  ExtensionContext,
} from "@fleetagent/pi-coding-agent";

const CUSTOM_TYPE = "grepple-metrics-v1";
const STATUS_KEY = "grepple-metrics";

interface RunState {
  runId: string;
  taskId: string;
  assignedCohort: string;
  explicit: boolean;
}

interface AnnotationEntry {
  schema: 1;
  event: "run_start" | "run_end" | "run_pause";
  runId: string;
  taskId: string;
  repository: string;
  revision: string;
  assignedCohort: string;
  outcome: "unknown" | "success" | "failure" | "abandoned";
  humanInterventions: number | null;
  evaluatorScore: number | null;
  rubric: string;
  regressions: number | null;
  firstEditSurvived: boolean | null;
}

interface EndArguments {
  outcome: AnnotationEntry["outcome"];
  evaluatorScore: number | null;
  rubric: string;
  humanInterventions: number | null;
  regressions: number | null;
  firstEditSurvived: boolean | null;
}

export default function agentMetrics(pi: ExtensionAPI): void {
  let active: RunState | null = null;

  pi.registerFlag("grepple-metrics", {
    description: "record content-free Grepple utility run boundaries",
    type: "boolean",
    default: false,
  });
  pi.registerFlag("grepple-metrics-cohort", {
    description: "assigned metrics cohort for automatic runs",
    type: "string",
    default: "unknown",
  });

  const enabled = (): boolean => pi.getFlag("grepple-metrics") === true;

  const setStatus = (ctx: ExtensionContext): void => {
    if (active === null) {
      ctx.ui.setStatus(STATUS_KEY, enabled() ? "metrics: ready" : undefined);
      return;
    }
    ctx.ui.setStatus(
      STATUS_KEY,
      `metrics: ${active.taskId || active.runId.slice(0, 8)} [${active.assignedCohort}]`,
    );
  };

  const revision = async (): Promise<string> => {
    try {
      const result = await pi.exec("git", ["rev-parse", "HEAD"]);
      return result.code === 0 ? result.stdout.trim() : "";
    } catch {
      return "";
    }
  };

  const start = async (
    taskId: string,
    assignedCohort: string,
    explicit: boolean,
    ctx: ExtensionContext,
  ): Promise<void> => {
    if (active !== null) {
      throw new Error(`metrics run ${active.runId} is already active`);
    }
    active = {
      runId: randomUUID(),
      taskId,
      assignedCohort: assignedCohort || "unknown",
      explicit,
    };
    const entry = await annotation(active, "run_start", ctx);
    pi.appendEntry(CUSTOM_TYPE, entry);
    setStatus(ctx);
  };

  const finish = async (values: EndArguments, ctx: ExtensionContext): Promise<void> => {
    if (active === null) {
      throw new Error("no metrics run is active");
    }
    const entry = await annotation(active, "run_end", ctx);
    entry.outcome = values.outcome;
    entry.evaluatorScore = values.evaluatorScore;
    entry.rubric = values.rubric;
    entry.humanInterventions = values.humanInterventions;
    entry.regressions = values.regressions;
    entry.firstEditSurvived = values.firstEditSurvived;
    pi.appendEntry(CUSTOM_TYPE, entry);
    active = null;
    setStatus(ctx);
  };

  const annotation = async (
    state: RunState,
    event: AnnotationEntry["event"],
    ctx: ExtensionContext,
  ): Promise<AnnotationEntry> => ({
    schema: 1,
    event,
    runId: state.runId,
    taskId: state.taskId,
    repository: basename(ctx.cwd),
    revision: await revision(),
    assignedCohort: state.assignedCohort,
    outcome: "unknown",
    humanInterventions: null,
    evaluatorScore: null,
    rubric: "",
    regressions: null,
    firstEditSurvived: null,
  });

  pi.registerCommand("metrics-start", {
    description: "start a metrics task: /metrics-start TASK_ID COHORT",
    handler: async (args: string, ctx: ExtensionCommandContext): Promise<void> => {
      const [taskId = "", cohort = "unknown"] = args.trim().split(/\s+/);
      if (taskId === "") {
        ctx.ui.notify("usage: /metrics-start TASK_ID COHORT", "warning");
        return;
      }
      try {
        await start(taskId, cohort, true, ctx);
        ctx.ui.notify(`metrics task ${taskId} started`, "info");
      } catch (error) {
        ctx.ui.notify(error instanceof Error ? error.message : String(error), "error");
      }
    },
  });

  pi.registerCommand("metrics-end", {
    description:
      "end metrics task: /metrics-end OUTCOME [SCORE|-] [INTERVENTIONS|-] [REGRESSIONS|-] [SURVIVED|-] [RUBRIC|-]",
    handler: async (args: string, ctx: ExtensionCommandContext): Promise<void> => {
      const parsed = parseEndArguments(args);
      if (typeof parsed === "string") {
        ctx.ui.notify(parsed, "warning");
        return;
      }
      try {
        await finish(parsed, ctx);
        ctx.ui.notify(`metrics task ended: ${parsed.outcome}`, "info");
      } catch (error) {
        ctx.ui.notify(error instanceof Error ? error.message : String(error), "error");
      }
    },
  });

  pi.registerCommand("metrics-status", {
    description: "show the active metrics task",
    handler: async (_args: string, ctx: ExtensionCommandContext): Promise<void> => {
      if (active === null) {
        ctx.ui.notify(enabled() ? "metrics enabled; no active run" : "metrics disabled", "info");
        return;
      }
      ctx.ui.notify(
        `metrics ${active.runId}: task=${active.taskId || "unlabelled"} cohort=${active.assignedCohort}`,
        "info",
      );
    },
  });

  pi.on("session_start", async (event, ctx) => {
    active = restoreState(ctx);
    if (event.reason === "fork" && active !== null) {
      const inherited = active;
      active = null;
      await start(inherited.taskId, inherited.assignedCohort, inherited.explicit, ctx);
      return;
    }
    setStatus(ctx);
  });

  pi.on("session_tree", async (_event, ctx) => {
    const branchActive = restoreState(ctx);
    if (active === null || branchActive !== null) {
      active = branchActive;
      setStatus(ctx);
      return;
    }
    const inherited = active;
    active = null;
    await start(inherited.taskId, inherited.assignedCohort, inherited.explicit, ctx);
  });

  pi.on("before_agent_start", async (_event, ctx) => {
    if (enabled() && active === null) {
      const cohort = String(pi.getFlag("grepple-metrics-cohort") || "unknown");
      await start("", cohort, false, ctx);
    }
  });

  pi.on("agent_settled", async (_event, ctx) => {
    if (active !== null && !active.explicit) {
      await finish(
        {
          outcome: "unknown",
          evaluatorScore: null,
          rubric: "",
          humanInterventions: null,
          regressions: null,
          firstEditSurvived: null,
        },
        ctx,
      );
    }
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    if (active === null) {
      return;
    }
    const entry = await annotation(active, "run_pause", ctx);
    pi.appendEntry(CUSTOM_TYPE, entry);
  });
}

function restoreState(ctx: ExtensionContext): RunState | null {
  let restored: RunState | null = null;
  for (const entry of ctx.session.getBranch()) {
    if (entry.type !== "custom" || entry.customType !== CUSTOM_TYPE) {
      continue;
    }
    const data = entry.data as Partial<AnnotationEntry>;
    if (data.schema !== 1 || typeof data.runId !== "string") {
      continue;
    }
    if (data.event === "run_start") {
      restored = {
        runId: data.runId,
        taskId: data.taskId || "",
        assignedCohort: data.assignedCohort || "unknown",
        explicit: data.taskId !== "",
      };
    } else if (data.event === "run_end" && restored?.runId === data.runId) {
      restored = null;
    }
  }
  return restored;
}

export function parseEndArguments(args: string): EndArguments | string {
  const [
    outcome = "",
    score = "-",
    interventions = "-",
    regressions = "-",
    survived = "-",
    rubric = "-",
  ] = args.trim().split(/\s+/);
  if (!(["success", "failure", "abandoned"] as string[]).includes(outcome)) {
    return "outcome must be success, failure, or abandoned";
  }
  const evaluatorScore = optionalNumber(score);
  const humanInterventions = optionalInteger(interventions);
  const regressionCount = optionalInteger(regressions);
  const firstEditSurvived = optionalBoolean(survived);
  if (evaluatorScore === undefined || humanInterventions === undefined || regressionCount === undefined || firstEditSurvived === undefined) {
    return "score must be numeric; interventions/regressions integers; survived true, false, or -";
  }
  return {
    outcome: outcome as EndArguments["outcome"],
    evaluatorScore,
    rubric: rubric === "-" ? "" : rubric,
    humanInterventions,
    regressions: regressionCount,
    firstEditSurvived,
  };
}

function optionalNumber(value: string): number | null | undefined {
  if (value === "-") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function optionalInteger(value: string): number | null | undefined {
  const parsed = optionalNumber(value);
  if (parsed === null || parsed === undefined) return parsed;
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : undefined;
}

function optionalBoolean(value: string): boolean | null | undefined {
  if (value === "-") return null;
  if (value === "true") return true;
  if (value === "false") return false;
  return undefined;
}
