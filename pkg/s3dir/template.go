package s3dir

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"embed"
	"errors"
	"github.com/andybalholm/brotli"
	"github.com/aws/smithy-go"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
)

var (
	//go:embed templates/* templates/layouts/*
	files     embed.FS
	templates = make(map[string]*template.Template)
)

// Initialise all application templates on startup
func init() {
	templateFiles, err := fs.ReadDir(files, "templates")
	if err != nil {
		panic(err)
	}

	for _, t := range templateFiles {
		if t.IsDir() {
			continue
		}

		pt, err := template.
			New(t.Name()).
			Funcs(templateFunctions).
			ParseFS(files,
				path.Join("templates", t.Name()),
				path.Join("templates", "layouts", "*.tmpl"),
			)

		if err != nil {
			panic(err)
		}

		templates[t.Name()] = pt
	}
}

type Renderer struct {
	// Title is the page title
	Title string
}

// RenderTemplateCode renders an HTML template to the target response using the supplied status code.
// It uses compression where requested by the client.
func (r *Renderer) RenderTemplateCode(code int, rw http.ResponseWriter, req *http.Request, t *template.Template, ctx map[string]any) {
	// Create template context specific to the render.
	// Combination of input context and global renderer context.
	localCtx := map[string]any{
		"Title": r.Title,
	}

	if len(ctx) > 0 {
		for k, v := range ctx {
			localCtx[k] = v
		}
	}

	var b bytes.Buffer
	err := t.Execute(&b, localCtx)
	if err != nil {
		log.Println(err)
	}

	rw.Header().Set("Content-Type", "text/html")

	// Handle response compression
encoding:
	for _, enc := range strings.Split(req.Header.Get("Accept-Encoding"), ",") {
		switch encoder := strings.TrimSpace(enc); encoder {
		case "gzip", "x-gzip":
			rw.Header().Set("Content-Encoding", encoder)
			rw.Header().Add("Vary", "Accept-Encoding")

			var w bytes.Buffer
			gw := gzip.NewWriter(&w)
			_, _ = b.WriteTo(gw)
			_ = gw.Close()

			b.Reset()
			b = w

			break encoding
		case "deflate":
			rw.Header().Set("Content-Encoding", encoder)
			rw.Header().Add("Vary", "Accept-Encoding")

			var w bytes.Buffer
			fw, _ := flate.NewWriter(&w, flate.DefaultCompression)
			_, _ = b.WriteTo(fw)
			_ = fw.Close()

			b.Reset()
			b = w

			break encoding
		case "br":
			rw.Header().Set("Content-Encoding", encoder)
			rw.Header().Add("Vary", "Accept-Encoding")

			var w bytes.Buffer
			bw := brotli.NewWriter(&w)
			_, _ = b.WriteTo(bw)
			_ = bw.Close()

			b.Reset()
			b = w

			break encoding
		}
	}

	rw.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	rw.WriteHeader(code)
	_, _ = b.WriteTo(rw)
}

func (r *Renderer) RenderTemplate(rw http.ResponseWriter, req *http.Request, t *template.Template, ctx map[string]any) {
	r.RenderTemplateCode(http.StatusOK, rw, req, t, ctx)
}

func (r *Renderer) RenderError(rw http.ResponseWriter, req *http.Request, err error, segments ...string) {
	var errorCode = "Error"
	var errorMessage = err.Error()

	var ae smithy.APIError
	if errors.As(err, &ae) {
		errorCode = ae.ErrorCode()
		errorMessage = ae.ErrorMessage()
	}

	r.RenderTemplateCode(http.StatusInternalServerError, rw, req, templates["error.tmpl"], map[string]any{
		"Segments": segments,
		"Code":     errorCode,
		"Message":  errorMessage,
	})
}
