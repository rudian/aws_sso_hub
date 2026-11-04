//go:build server || mcp

package main

// Server and MCP modes would expose every bound service method over an unauthenticated network listener.
// This application is a desktop app only, so those build tags are refused: the identifier below does not exist,
// and the compiler stops with its name.
var _ = serverAndMcpModesAreNotSupportedByThisApplication
