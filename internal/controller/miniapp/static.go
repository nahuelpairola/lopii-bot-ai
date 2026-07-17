package miniapp

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFS embed.FS

// registerStatic mounts the vendored picocss/Chart.js/app.css/app.js at
// /app/static/*, served straight from the embedded binary — no CDN, no
// separate deploy step (go:embed bakes these into the compiled binary).
func registerStatic(r *gin.RouterGroup) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded FS is compiled in — a read failure here is a build bug, not a runtime one
	}
	r.StaticFS("/static", http.FS(sub))
}
