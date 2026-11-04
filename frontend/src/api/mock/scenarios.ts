// Scenario builders live in world.ts next to the helpers they share; this module is the documented entry point.
import type { ScenarioId } from '../../lib/initialUI'
export { buildScenario, buildFresh, buildMid, buildDone, buildEmpty, buildEdge, resetWorld } from './world'

export const SCENARIO_IDS: readonly ScenarioId[] = ['fresh', 'mid', 'done', 'empty', 'edge']
