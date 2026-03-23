package web

import "embed"

// WebAssets holds the embedded web UI files. It is populated at build time
// by the embed_gen.go file at the repository root, which is generated or
// maintained manually. As a fallback, this package provides an empty FS
// so the server compiles without embedded assets.
//
// To wire in the real assets, the top-level web_embed.go uses go:embed
// and assigns to this variable via an init function.
var WebAssets embed.FS
