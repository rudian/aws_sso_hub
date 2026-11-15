// Package version holds the product identity constants.
//
// BundleID and the data folder name (paths.AppDirName) predate the rename to Codepass and must never change:
// the BundleID is part of the updater's code requirement and of the single-instance ID, and the data folder
// holds every user's existing settings and backups.
package version

const Version = "0.0.1"
const ProductName = "Codepass"
const BundleID = "com.rudian.awsssohub"
