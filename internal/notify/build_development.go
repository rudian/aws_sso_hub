//go:build !production

package notify

// productionBuild is false in development builds: real notifications there need AWSSSOHUB_REAL_NOTIFICATIONS=1.
const productionBuild = false
