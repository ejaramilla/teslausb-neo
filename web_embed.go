package teslausbneo

import (
	"embed"

	"github.com/ejaramilla/teslausb-neo/internal/web"
)

//go:embed all:web
var embeddedWebAssets embed.FS

func init() {
	web.WebAssets = embeddedWebAssets
}
