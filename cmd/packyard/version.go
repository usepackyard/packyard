package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// runVersion prints the build metadata in a shell-friendly format:
//
//	packyard version <tag-or-sha>
//	  commit:  <vcs.revision>
//	  built:   <vcs.time>
//	  go:      <runtime.Version>
//	  os/arch: <GOOS/GOARCH>
//
// `packyard version --short` prints only the version string, suitable
// for scripting (`VER=$(packyard version --short)`).
func runVersion(args []string) int {
	short := false
	for _, a := range args {
		if a == "--short" || a == "-s" {
			short = true
		}
	}

	if short {
		fmt.Println(version)
		return 0
	}

	fmt.Printf("packyard version %s\n", version)

	// debug.ReadBuildInfo pulls VCS metadata embedded by the Go toolchain
	// when the binary is built from a git checkout with VCS stamping
	// enabled (default for `go build`). Missing for `go run` builds.
	info, ok := debug.ReadBuildInfo()
	if ok {
		var commit, built string
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.time":
				built = s.Value
			}
		}
		if commit != "" {
			fmt.Printf("  commit:  %s\n", commit)
		}
		if built != "" {
			fmt.Printf("  built:   %s\n", built)
		}
	}
	fmt.Printf("  go:      %s\n", runtime.Version())
	fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return 0
}
