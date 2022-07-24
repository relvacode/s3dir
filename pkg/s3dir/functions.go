package s3dir

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

var byteSizes = []string{"Bytes", "KB", "MB", "GB", "TB"}

var templateFunctions = template.FuncMap{
	"last": func(i, size int) bool { return i == size-1 },
	"joinUrl": func(segments []string, extra string) string {
		var b strings.Builder
		for i := 0; i < len(segments); i++ {
			b.WriteRune('/')
			b.WriteString(segments[i])
		}

		b.WriteRune('/')
		b.WriteString(extra)

		return b.String()
	},
	"joinSegmentUrl": func(index int, segments []string) string {
		var b strings.Builder

		for i := 0; (index < 0 || i < index+1) && i < len(segments); i++ {
			b.WriteRune('/')
			b.WriteString(segments[i])
		}

		return b.String()
	},

	"trimSegmentPrefix": func(segments []string, key string) string {
		var keyComponents = strings.Split(key, "/")

		var x int
		for i := 0; i < len(segments) && x < len(keyComponents); i++ {
			if segments[i] == keyComponents[x] {
				keyComponents = keyComponents[1:]
				continue
			}
		}

		return strings.Join(keyComponents, "/")
	},

	"formatBytes": func(bytes int64) string {
		if bytes == 0 {
			return "0 Bytes"
		}

		var i = int64(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))
		return fmt.Sprintf("%.2f %s", float64(bytes)/math.Pow(1024, float64(i)), byteSizes[i])
	},

	"formatTime": func(t time.Time) string {
		return t.Format(time.ANSIC)
	},
}
