package web

import "embed"

// Dist is the built Svelte admin panel. The Docker build replaces this directory.
//go:embed all:build
var Dist embed.FS
