package miniapp

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"lopiibot.com/internal/controller/miniapp/templates"
)

//go:embed static/*
var staticFS embed.FS

type staticAsset struct {
	contentType string
	raw         []byte
	gzipped     []byte
}

var staticAssets = buildStaticAssets()

func buildStaticAssets() map[string]staticAsset {
	entries, err := fs.ReadDir(staticFS, "static")
	if err != nil {
		panic(err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	assets := make(map[string]staticAsset, len(names))
	digest := sha256.New()

	for _, name := range names {
		raw, err := staticFS.ReadFile("static/" + name)
		if err != nil {
			panic(err)
		}
		digest.Write([]byte(name))
		digest.Write(raw)

		assets[name] = staticAsset{
			contentType: contentTypeFor(name),
			raw:         raw,
			gzipped:     gzipIfSmaller(raw),
		}
	}

	templates.AssetVersion = hex.EncodeToString(digest.Sum(nil))[:12]
	return assets
}

func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func gzipIfSmaller(raw []byte) []byte {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil
	}
	if _, err := w.Write(raw); err != nil {
		return nil
	}
	if err := w.Close(); err != nil {
		return nil
	}
	if buf.Len() >= len(raw) {
		return nil
	}
	return buf.Bytes()
}

func registerStatic(r *gin.RouterGroup) {
	r.GET("/static/:name", serveStatic)
}

func serveStatic(ctx *gin.Context) {
	asset, ok := staticAssets[ctx.Param("name")]
	if !ok {
		ctx.Status(http.StatusNotFound)
		return
	}

	header := ctx.Writer.Header()
	if ctx.Query(templates.AssetVersionParam) == templates.AssetVersion {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		header.Set("Cache-Control", "no-cache")
	}

	if asset.gzipped != nil && strings.Contains(ctx.GetHeader("Accept-Encoding"), "gzip") {
		header.Set("Content-Encoding", "gzip")
		header.Set("Vary", "Accept-Encoding")
		ctx.Data(http.StatusOK, asset.contentType, asset.gzipped)
		return
	}

	ctx.Data(http.StatusOK, asset.contentType, asset.raw)
}
