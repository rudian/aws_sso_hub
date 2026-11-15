package projects

import "runtime"

// pathFold: paths compare case-insensitively on macOS and Windows (default volumes).
var pathFold = runtime.GOOS != "linux"
