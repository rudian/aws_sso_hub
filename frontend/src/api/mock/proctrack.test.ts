import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockProctrackService, resetMockProctrack } from "./proctrack";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

async function call<T>(p: Promise<T>): Promise<T> {
  const done = p.then((v) => ({ v }));
  await vi.advanceTimersByTimeAsync(5000);
  return (await done).v;
}

describe("mock proctrack service", () => {
  it("starts empty", async () => {
    resetMockProctrack("");
    expect(await call(mockProctrackService.List())).toEqual([]);
    expect((await call(mockProctrackService.Leftovers("nope"))).count).toBe(0);
  });

  it("lists two closed sessions and Force stop removes one", async () => {
    resetMockProctrack("?leftover=2");
    const list = await call(mockProctrackService.List());
    expect(list.map((s) => [s.session, s.count, s.live])).toEqual([
      ["task-acme-web-1", 3, false],
      ["task-globex-api-2", 1, false],
    ]);
    const res = await call(mockProctrackService.ForceStop("task-acme-web-1"));
    expect(res).toMatchObject({
      found: 3,
      stopped: 3,
      names: ["esbuild", "node", "sh"],
      survivors: [],
    });
    expect(
      (await call(mockProctrackService.List())).map((s) => s.session),
    ).toEqual(["task-globex-api-2"]);
  });

  it("a program that cannot be stopped is reported with its PID", async () => {
    resetMockProctrack("?leftover=stuck");
    const res = await call(mockProctrackService.ForceStop("task-acme-web-1"));
    expect(res.stopped).toBe(2);
    expect(res.survivors.map((p) => p.pid)).toEqual([48211]);
    expect(
      (await call(mockProctrackService.Leftovers("task-acme-web-1"))).count,
    ).toBe(1);
  });

  it("command lines come on demand and name the program as Go does", async () => {
    resetMockProctrack("?leftover=2");
    const list = await call(mockProctrackService.List());
    expect(list[0].procs[0]).toMatchObject({
      name: "node",
      command: "/usr/local/bin/node",
    });
    const cmds = await call(mockProctrackService.Commands("task-acme-web-1"));
    expect(cmds.find((c) => c.pid === 48211)?.command).toBe(
      "node ./node_modules/.bin/vite --port 5173",
    );
    expect(await call(mockProctrackService.Commands("nope"))).toEqual([]);
  });

  it("Force stop all empties the list", async () => {
    resetMockProctrack("?leftover=2");
    const all = await call(mockProctrackService.ForceStopAll());
    expect(all).toHaveLength(2);
    expect(await call(mockProctrackService.List())).toEqual([]);
  });
});
