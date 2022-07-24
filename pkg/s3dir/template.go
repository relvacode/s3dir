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

// RenderTemplateCode renders an HTML template to the target response using the supplied status code.
// It uses compression where requested by the client.
func RenderTemplateCode(code int, rw http.ResponseWriter, r *http.Request, t *template.Template, ctx any) {
	var b bytes.Buffer
	err := t.Execute(&b, ctx) // TODO do something with err
	if err != nil {
		log.Println(err)
	}

	rw.Header().Set("Content-Type", "text/html")

	// Handle response compression
encoding:
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
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

func RenderTemplate(rw http.ResponseWriter, r *http.Request, t *template.Template, ctx any) {
	RenderTemplateCode(http.StatusOK, rw, r, t, ctx)
}

func RenderError(rw http.ResponseWriter, r *http.Request, err error, segments ...string) {
	var errorCode = "Error"
	var errorMessage = err.Error()

	var ae smithy.APIError
	if errors.As(err, &ae) {
		errorCode = ae.ErrorCode()
		errorMessage = ae.ErrorMessage()
	}

	RenderTemplateCode(http.StatusInternalServerError, rw, r, templates["error.tmpl"], map[string]any{
		"Segments": segments,
		"Code":     errorCode,
		"Message":  errorMessage,
	})
}
