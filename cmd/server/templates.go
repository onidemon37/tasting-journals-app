package main

import "embed"

//go:embed templates/*.html
var templatesFS embed.FS
