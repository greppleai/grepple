import assert from "node:assert/strict";
import test from "node:test";
import agentMetrics, { parseEndArguments } from "../extensions/agent-metrics.ts";

interface CommandRegistration {
  handler: (args: string, context: any) => Promise<void>;
}

function harness(entries: any[] = []) {
  let branchEntries = entries;
  const commands = new Map<string, CommandRegistration>();
  const events = new Map<string, Array<(event: any, context: any) => unknown>>();
  const appended: Array<{ customType: string; data: any }> = [];
  const flags = new Map<string, boolean | string>();
  const notifications: string[] = [];
  const statuses: Array<string | undefined> = [];
  const pi: any = {
    registerFlag(name: string, options: any) {
      flags.set(name, options.default);
    },
    getFlag(name: string) {
      return flags.get(name);
    },
    registerCommand(name: string, options: CommandRegistration) {
      commands.set(name, options);
    },
    on(name: string, handler: (event: any, context: any) => unknown) {
      const handlers = events.get(name) || [];
      handlers.push(handler);
      events.set(name, handlers);
    },
    appendEntry(customType: string, data: any) {
      appended.push({ customType, data });
    },
    async exec() {
      return { code: 0, stdout: "abc123\n", stderr: "" };
    },
  };
  const context: any = {
    cwd: "/private/work/repo",
    session: {
      getEntries: () => entries,
      getBranch: () => branchEntries,
    },
    ui: {
      notify(message: string) {
        notifications.push(message);
      },
      setStatus(_key: string, value: string | undefined) {
        statuses.push(value);
      },
    },
  };
  agentMetrics(pi);
  return {
    commands,
    events,
    appended,
    flags,
    notifications,
    statuses,
    context,
    setBranch(next: any[]) {
      branchEntries = next;
    },
  };
}

test("explicit task writes only content-free versioned annotations", async () => {
  const state = harness();
  await state.commands.get("metrics-start")!.handler("task-7 grepple", state.context);
  await state.commands.get("metrics-end")!.handler("success 4.5 1 0 true", state.context);
  assert.equal(state.appended.length, 2);
  assert.equal(state.appended[0].customType, "grepple-metrics-v1");
  assert.deepEqual(
    {
      event: state.appended[0].data.event,
      taskId: state.appended[0].data.taskId,
      cohort: state.appended[0].data.assignedCohort,
      repository: state.appended[0].data.repository,
      revision: state.appended[0].data.revision,
    },
    { event: "run_start", taskId: "task-7", cohort: "grepple", repository: "repo", revision: "abc123" },
  );
  assert.equal(state.appended[1].data.outcome, "success");
  assert.equal(state.appended[1].data.firstEditSurvived, true);
  const serialized = JSON.stringify(state.appended);
  assert.equal(serialized.includes("/private/work"), false);
  assert.equal(serialized.includes("prompt"), false);
  assert.equal(serialized.includes("content"), false);
});

test("automatic collection is opt-in and settles each logical run", async () => {
  const state = harness();
  state.flags.set("grepple-metrics", true);
  state.flags.set("grepple-metrics-cohort", "control");
  for (const handler of state.events.get("before_agent_start") || []) {
    await handler({}, state.context);
  }
  for (const handler of state.events.get("agent_settled") || []) {
    await handler({}, state.context);
  }
  assert.deepEqual(state.appended.map((entry) => entry.data.event), ["run_start", "run_end"]);
  assert.equal(state.appended[0].data.assignedCohort, "control");
});

test("end argument validation preserves missing values", () => {
  assert.equal(typeof parseEndArguments("success nope"), "string");
  assert.deepEqual(parseEndArguments("failure - 2 1 false"), {
    outcome: "failure",
    evaluatorScore: null,
    rubric: "",
    humanInterventions: 2,
    regressions: 1,
    firstEditSurvived: false,
  });
});

test("explicit state restores across resume and flushes an incomplete pause", async () => {
  const state = harness([
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: {
        schema: 1,
        event: "run_start",
        runId: "restored-run",
        taskId: "restored-task",
        assignedCohort: "control",
      },
    },
  ]);
  for (const handler of state.events.get("session_start") || []) {
    await handler({}, state.context);
  }
  await state.commands.get("metrics-status")!.handler("", state.context);
  assert.equal(state.notifications.at(-1)?.includes("restored-task"), true);
  for (const handler of state.events.get("session_shutdown") || []) {
    await handler({}, state.context);
  }
  assert.equal(state.appended.at(-1)?.data.event, "run_pause");
  assert.equal(state.appended.at(-1)?.data.outcome, "unknown");
});

test("closed and future-schema entries do not restore active state", async () => {
  const state = harness([
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: { schema: 2, event: "run_start", runId: "future" },
    },
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: { schema: 1, event: "run_start", runId: "closed", taskId: "task" },
    },
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: { schema: 1, event: "run_end", runId: "closed" },
    },
  ]);
  for (const handler of state.events.get("session_start") || []) {
    await handler({}, state.context);
  }
  await state.commands.get("metrics-status")!.handler("", state.context);
  assert.equal(state.notifications.at(-1), "metrics disabled");
});

test("recorder is outside model context and agent-visible tool accounting", async () => {
  const state = harness();
  assert.equal(state.events.has("context"), false);
  assert.equal(state.events.has("tool_call"), false);
  assert.equal(state.events.has("tool_result"), false);
  assert.deepEqual(
    [...state.commands.keys()].sort(),
    ["metrics-end", "metrics-start", "metrics-status"],
  );
  await state.commands
    .get("metrics-start")!
    .handler("opaque-task control raw prompt and edit content", state.context);
  const serialized = JSON.stringify(state.appended);
  assert.equal(serialized.includes("raw prompt"), false);
  assert.equal(serialized.includes("edit content"), false);
});

test("forking creates a distinct logical run identity", async () => {
  const state = harness([
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: {
        schema: 1,
        event: "run_start",
        runId: "inherited-run",
        taskId: "paired-task",
        assignedCohort: "grepple",
      },
    },
  ]);
  for (const handler of state.events.get("session_start") || []) {
    await handler({ reason: "fork" }, state.context);
  }
  assert.equal(state.appended.length, 1);
  assert.equal(state.appended[0].data.event, "run_start");
  assert.equal(state.appended[0].data.taskId, "paired-task");
  assert.notEqual(state.appended[0].data.runId, "inherited-run");
});

test("tree navigation re-anchors an active task on the selected branch", async () => {
  const state = harness();
  await state.commands.get("metrics-start")!.handler("tree-task control", state.context);
  const originalRunId = state.appended[0].data.runId;

  state.setBranch([]);
  for (const handler of state.events.get("session_tree") || []) {
    await handler({}, state.context);
  }

  assert.equal(state.appended.length, 2);
  assert.equal(state.appended[1].data.event, "run_start");
  assert.equal(state.appended[1].data.taskId, "tree-task");
  assert.equal(state.appended[1].data.assignedCohort, "control");
  assert.notEqual(state.appended[1].data.runId, originalRunId);
});

test("resume ignores run state from an abandoned branch", async () => {
  const state = harness([
    {
      type: "custom",
      customType: "grepple-metrics-v1",
      data: {
        schema: 1,
        event: "run_start",
        runId: "abandoned-run",
        taskId: "abandoned-task",
        assignedCohort: "control",
      },
    },
  ]);
  state.setBranch([]);

  for (const handler of state.events.get("session_start") || []) {
    await handler({ reason: "resume" }, state.context);
  }
  await state.commands.get("metrics-status")!.handler("", state.context);

  assert.equal(state.notifications.at(-1), "metrics disabled");
});
