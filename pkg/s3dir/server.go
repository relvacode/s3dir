package s3dir

import (
	"archive/zip"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

func trimPathSegments(path string) (segments []string) {
	var lastCut int
	for i, chr := range path {
		if chr != '/' {
			continue
		}

		// Ignore contiguous cuts
		if i-lastCut < 2 {
			lastCut = i
			continue
		}

		segments = append(segments, path[lastCut+1:i])
		lastCut = i

	}

	if lastCut < len(path)-1 {
		segments = append(segments, path[lastCut+1:])
	}

	return segments
}

func New(s3Client *s3.Client, rd *Renderer) *Server {
	return &Server{
		s3: s3Client,
		rd: rd,
	}
}

type Server struct {
	s3 *s3.Client
	rd *Renderer
}

func (s *Server) ListBuckets(rw http.ResponseWriter, r *http.Request) {
	response, err := s.s3.ListBuckets(r.Context(), new(s3.ListBucketsInput))
	if err != nil {
		s.rd.RenderError(rw, r, err)
		return
	}

	s.rd.RenderTemplate(rw, r, templates["buckets.tmpl"], map[string]any{
		"Segments": []string{},
		"Buckets":  response.Buckets,
	})
}

func segmentPrefix(segments []string) string {
	if len(segments) == 0 {
		return ""
	}

	var b strings.Builder
	for _, segment := range segments {
		b.WriteString(segment)
		b.WriteRune('/')
	}

	return b.String()
}

type ObjectSortFunc func(a, b types.Object) bool

func sortObjectsByLastModified(a, b types.Object) bool {
	switch {
	case a.LastModified == nil && b.LastModified == nil:
		return false
	case a.LastModified == nil:
		return true
	case b.LastModified == nil:
		return false
	default:
		return a.LastModified.After(*b.LastModified)
	}
}

func sortObjectsBySize(a, b types.Object) bool {
	return a.Size > b.Size
}

func sortObjectsByName(a, b types.Object) bool {
	var (
		pa = path.Base(*a.Key)
		pb = path.Base(*b.Key)
	)

	return pa < pb
}

type SortOption struct {
	Name  string
	Value string
}

func (s *Server) ListBucketObjects(rw http.ResponseWriter, r *http.Request, bucket string, segments []string) {
	var params = s3.ListObjectsV2Input{
		Bucket:    &bucket,
		Delimiter: aws.String("/"),
		Prefix:    aws.String(segmentPrefix(segments)),
	}

	var objects []types.Object
	var prefixes []string
	var prefixKnown = make(map[string]struct{})

	for {
		response, err := s.s3.ListObjectsV2(r.Context(), &params)
		if err != nil {
			s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
			return
		}

		objects = append(objects, response.Contents...)

		for _, prefix := range response.CommonPrefixes {
			unescape, _ := url.PathUnescape(*prefix.Prefix)
			if _, ok := prefixKnown[unescape]; ok {
				continue
			}

			prefixKnown[unescape] = struct{}{}
			prefixes = append(prefixes, unescape)
		}

		if !response.IsTruncated {
			break
		}

		params.ContinuationToken = response.ContinuationToken
	}

	var sortFunc ObjectSortFunc
	var noSortCookie bool

	_, sortInQuery := r.URL.Query()["sort"]
	var sortByField = r.URL.Query().Get("sort")
	if !sortInQuery {
		noSortCookie = true
		c, err := r.Cookie("sort")
		if err == nil {
			sortByField = c.Value
		}
	}

	switch sortByField {
	case "lastModified":
		sortFunc = sortObjectsByLastModified
	case "name":
		sortFunc = sortObjectsByName
	case "size":
		sortFunc = sortObjectsBySize
	}

	if sortFunc != nil {
		slices.SortFunc(objects, sortFunc)
	}

	if !noSortCookie {
		sortCookie := http.Cookie{
			Name:     "sort",
			Value:    sortByField,
			HttpOnly: true,
			Path:     "/",
			Expires:  time.Unix(2147483647, 0), // Max cookie expiration time according to RFC2965 (32bit integer)
		}

		http.SetCookie(rw, &sortCookie)
	}

	s.rd.RenderTemplate(rw, r, templates["objects.tmpl"], map[string]any{
		"Segments":           append([]string{bucket}, segments...),
		"QueryString":        r.URL.RawQuery,
		"Objects":            objects,
		"Prefixes":           prefixes,
		"SelectedSortOption": sortByField,
		"SortOptions": []SortOption{
			{
				Name:  "",
				Value: "",
			},
			{
				Name:  "Name",
				Value: "name",
			},
			{
				Name:  "Last Modified",
				Value: "lastModified",
			},
			{
				Name:  "Size",
				Value: "size",
			},
		},
	})
}

func (s *Server) GetBucketObjectLocation(rw http.ResponseWriter, r *http.Request, bucket string, segments []string) {
	var key = strings.Join(segments, "/")
	signedUrl, err := s3.NewPresignClient(s.s3).PresignGetObject(r.Context(), &s3.GetObjectInput{
		Bucket:                     &bucket,
		Key:                        &key,
		ResponseContentDisposition: aws.String(fmt.Sprintf("inline; filename=%q", segments[len(segments)-1])),
	})

	if err != nil {
		s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
		return
	}

	rw.Header().Set("Location", signedUrl.URL)
	rw.WriteHeader(http.StatusTemporaryRedirect)
}

func (s *Server) BucketObject(rw http.ResponseWriter, r *http.Request, bucket string, segments []string) {
	var key = strings.Join(segments, "/")
	response, err := s.s3.HeadObject(r.Context(), &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})

	if err != nil {
		s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
		return
	}

	s.rd.RenderTemplate(rw, r, templates["object.tmpl"], map[string]any{
		"Segments": append([]string{bucket}, segments...),
		"Name":     segments[len(segments)-1],
		"Bucket":   bucket,
		"Key":      key,
		"Object":   response,
	})
}

func (s *Server) ZipArchive(rw http.ResponseWriter, r *http.Request, bucket string, segments []string) {
	var params = s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: aws.String(strings.Join(segments, "/")),
	}

	var filename = bucket
	if len(segments) > 0 {
		strings.Join(segments, "_")
	}

	var sw = NewStream(rw, func(rw http.ResponseWriter) {
		rw.Header().Set("Content-Type", "application/zip")
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("%s.zip", filename)))
		rw.WriteHeader(http.StatusOK)
	})

	var zw = zip.NewWriter(sw)

	for {
		response, err := s.s3.ListObjectsV2(r.Context(), &params)
		if err != nil {
			sw.Abort(func(rw http.ResponseWriter) {
				s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
			})
			return
		}

		for _, object := range response.Contents {
			objectKeySegments := trimPathSegments(*object.Key)
			objectKeySegments = objectKeySegments[len(segments):]

			signedUrl, _ := s3.NewPresignClient(s.s3).PresignGetObject(r.Context(), &s3.GetObjectInput{
				Bucket: &bucket,
				Key:    object.Key,
			})

			w, err := zw.CreateHeader(&zip.FileHeader{
				Name:     strings.Join(objectKeySegments, "/"),
				Modified: *object.LastModified,
				Method:   zip.Deflate,
			})
			if err != nil {
				sw.Abort(func(rw http.ResponseWriter) {
					s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
				})
				return
			}

			resp, err := http.DefaultClient.Get(signedUrl.URL)
			if err != nil {
				sw.Abort(func(rw http.ResponseWriter) {
					s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
				})
				return
			}

			_, err = io.Copy(w, resp.Body)
			_ = resp.Body.Close()

			if err != nil {
				sw.Abort(func(rw http.ResponseWriter) {
					s.rd.RenderError(rw, r, err, append([]string{bucket}, segments...)...)
				})
				return
			}
		}

		if !response.IsTruncated {
			break
		}

		params.ContinuationToken = response.ContinuationToken
	}

	_ = zw.Close()
	sw.Complete()
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Handle well-known paths
	switch r.URL.Path {
	case "/favicon.ico":
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	var segments = trimPathSegments(r.URL.Path)
	switch len(segments) {
	case 0:
		s.ListBuckets(rw, r)
		return
	default:
		if strings.HasSuffix(r.URL.Path, "/") {
			if _, ok := r.URL.Query()["archive"]; ok {
				s.ZipArchive(rw, r, segments[0], segments[1:])
				return
			}

			s.ListBucketObjects(rw, r, segments[0], segments[1:])
			return
		}

		if _, ok := r.URL.Query()["location"]; ok {
			s.GetBucketObjectLocation(rw, r, segments[0], segments[1:])
			return
		}

		s.BucketObject(rw, r, segments[0], segments[1:])
		return
	}
}
