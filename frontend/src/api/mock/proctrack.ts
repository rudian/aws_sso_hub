// Mock of ProctrackService (A2-13; plan 4.8b, design 5.6a): the programs that closed sessions left running, and Force stop.
//
// The Go service is not registered yet (A2-W does that), so wails3 has not generated its module or its types. Until then
// the shapes below mirror internal/api/proctrack.go field for field. A2-W replaces them with
//   import type { SessionLeftovers, ForceStopResult, LeftoverProcess } from '../models'
//   import * as ProctrackReal from '../../../bindings/github.com/rudian/codepass/internal/app/proctrackservice'
// and types the export as `typeof ProctrackReal`.
//
// URL switches (mock mode only):
//   ?leftover=2        two closed sessions with programs (the shot "ai-leftover"; default: none)
//   ?leftover=stuck    like 2, but one program of the first session cannot be stopped
import { CancellablePromise } from "@wailsio/runtime";
import { mockEmit } from "../events";
import { MIN, ago, deepCopy } from "./clock";
import { READ, SLOW, run } from "./util";

export interface LeftoverProcess {
  pid: number;
  ppid: number;
  name: string;
  command?: string;
  cpu: number;
  memory: number;
}
export interface SessionLeftovers {
  session: string;
  leader: number;
  live: boolean;
  closedAt?: string;
  count: number;
  cpu: number;
  memory: number;
  procs: LeftoverProcess[];
}
export interface ForceStopResult {
  session: string;
  found: number;
  stopped: number;
  names: string[];
  survivors: LeftoverProcess[];
  error?: string;
}
export interface ProcessCommand {
  pid: number;
  command: string;
}

export type LeftoverScenario = "none" | "two" | "stuck";

const MB = 1024 * 1024;

function sessions(s: LeftoverScenario): SessionLeftovers[] {
  if (s === "none") return [];
  const mk = (
    session: string,
    minutes: number,
    procs: LeftoverProcess[],
  ): SessionLeftovers => ({
    session,
    leader: 0,
    live: false,
    closedAt: ago(minutes * MIN),
    count: procs.length,
    cpu: procs.reduce((a, p) => a + p.cpu, 0),
    memory: procs.reduce((a, p) => a + p.memory, 0),
    procs,
  });
  // What Go sends: name is the program's own name, command is `comm` as ps prints it (argv[0], no arguments).
  // The full command lines of the expanded list come from Commands(session), on demand.
  return [
    mk("task-acme-web-1", 12, [
      {
        pid: 48211,
        ppid: 1,
        name: "node",
        command: "/usr/local/bin/node",
        cpu: 38.5,
        memory: 212 * MB,
      },
      {
        pid: 48230,
        ppid: 48211,
        name: "esbuild",
        command: "/usr/local/lib/node_modules/esbuild/bin/esbuild",
        cpu: 2.4,
        memory: 64 * MB,
      },
      {
        pid: 48197,
        ppid: 1,
        name: "sh",
        command: "/bin/sh",
        cpu: 0,
        memory: 2 * MB,
      },
    ]),
    mk("task-globex-api-2", 34, [
      {
        pid: 47102,
        ppid: 1,
        name: "node",
        command: "/usr/local/bin/node",
        cpu: 0.4,
        memory: 96 * MB,
      },
    ]),
  ];
}

const COMMAND_LINES: Record<number, string> = {
  48211: "node ./node_modules/.bin/vite --port 5173",
  48230:
    "/usr/local/lib/node_modules/esbuild/bin/esbuild --service=0.25.4 --ping",
  48197: 'sh -c "npm run dev"',
  47102: "node server.js --port 4000",
};

let scenario: LeftoverScenario = "none";
let state: SessionLeftovers[] = [];
let stuckPid = 0;

/** Back to the state a fresh page load has (tests). */
export function resetMockProctrack(search = ""): void {
  const v = new URLSearchParams(search).get("leftover");
  scenario = v === "2" ? "two" : v === "stuck" ? "stuck" : "none";
  state = sessions(scenario);
  stuckPid = scenario === "stuck" ? state[0].procs[0].pid : 0;
}
resetMockProctrack(typeof window !== "undefined" ? window.location.search : "");

// 'proctrack:changed' joins EventPayloads in src/api/events.ts when A2-W registers the service.
const emitUntyped = mockEmit as (name: string, data: unknown) => void;

function changed(): void {
  emitUntyped("proctrack:changed", deepCopy(state));
}

function stop(session: string): ForceStopResult {
  const s = state.find((x) => x.session === session);
  if (!s) return { session, found: 0, stopped: 0, names: [], survivors: [] };
  const found = s.procs.length;
  const names = [...new Set(s.procs.map((p) => p.name))].sort();
  const survivors = s.procs.filter((p) => p.pid === stuckPid);
  if (survivors.length === 0)
    state = state.filter((x) => x.session !== session);
  else {
    s.procs = survivors;
    s.count = survivors.length;
    s.cpu = survivors.reduce((a, p) => a + p.cpu, 0);
    s.memory = survivors.reduce((a, p) => a + p.memory, 0);
  }
  changed();
  return {
    session,
    found,
    stopped: found - survivors.length,
    names,
    survivors: deepCopy(survivors),
  };
}

export interface ProctrackMock {
  List(): CancellablePromise<SessionLeftovers[]>;
  Leftovers(session: string): CancellablePromise<SessionLeftovers>;
  ForceStop(session: string): CancellablePromise<ForceStopResult>;
  ForceStopAll(): CancellablePromise<ForceStopResult[]>;
  Commands(session: string): CancellablePromise<ProcessCommand[]>;
}

export const mockProctrackService: ProctrackMock = {
  List: () => run(READ, () => state),
  Leftovers: (session) =>
    run(
      READ,
      () =>
        state.find((s) => s.session === session) ?? {
          session,
          leader: 0,
          live: false,
          count: 0,
          cpu: 0,
          memory: 0,
          procs: [],
        },
    ),
  // The real stop waits up to 5 s for a polite exit; the demo takes 1.5 s so the "Stopping..." row can be seen.
  ForceStop: (session) => run(1500, () => stop(session)),
  // One system call per expand in Go; display only.
  Commands: (session) =>
    run(READ, () =>
      (state.find((s) => s.session === session)?.procs ?? []).map((p) => ({
        pid: p.pid,
        command: COMMAND_LINES[p.pid] ?? p.command ?? p.name,
      })),
    ),
  ForceStopAll: () =>
    run(SLOW + 800, () => state.map((s) => s.session).map(stop)),
};
