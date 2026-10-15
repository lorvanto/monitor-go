// Package buildinfo provides the version shared by every application component.
package buildinfo

// Version is overridden with -ldflags -X by the build entry points.
var Version = "0.4.0-dev"

func UserAgent() string { return "MonitorGo/" + Version }
