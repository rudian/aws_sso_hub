/**
 * Oldest git the app works with. The backend (envcheck.GitMin) reports the same number as `min`; this is the
 * fallback for a report without it and the value the mock backend sends. Versions before 2.44.0 ignore
 * GIT_NO_LAZY_FETCH.
 */
export const GIT_MIN_VERSION = '2.44.0'
