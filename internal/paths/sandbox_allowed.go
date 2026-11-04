//go:build !production || sandbox

package paths

// sandboxAllowed is true in development builds and in builds made with the explicit "sandbox" tag: the
// AWSSSOHUB_SANDBOX environment variable may then redirect every path (and enable the fake service endpoints).
const sandboxAllowed = true
