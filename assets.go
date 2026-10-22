package main

import "embed"

//go:embed template/*.tmpl
var templateFiles embed.FS

//go:embed static
var staticFiles embed.FS
