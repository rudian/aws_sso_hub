//go:build production

package notify

// productionBuild is true in release builds (build/darwin/Taskfile.yml BUILD_FLAGS carries -tags production).
const productionBuild = true
