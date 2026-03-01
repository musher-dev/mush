package state

import "time"

// MCPServerStatus represents the connection and auth state of an MCP server.
type MCPServerStatus struct {
	Name          string
	Loaded        bool
	Authenticated bool
	Expired       bool
}

// Snapshot is an immutable status view consumed by UI renderers.
type Snapshot struct {
	Width  int
	Height int

	SidebarVisible   bool
	SidebarAvailable bool // Sidebar feature available per terminal capabilities (controls ^G hint; Ctrl+G re-probes if needed)
	SidebarWidth     int
	PaneXStart       int
	PaneWidth        int

	BundleLoadMode bool
	BundleName     string
	BundleVer      string
	BundleLayers   int
	BundleSkills   []string
	BundleAgents   []string
	BundleTools    []string
	BundleOther    []string

	HabitatID          string
	QueueID            string
	SupportedHarnesses []string

	StatusLabel string

	CopyMode bool
	JobID    string

	LastHeartbeat time.Time
	Completed     int
	Failed        int

	LastError     string
	LastErrorTime time.Time

	MCPServers []MCPServerStatus

	Now time.Time
}
