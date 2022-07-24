package s3dir

import "testing"

type trimPathSegmentTestCase struct {
	Using  string
	Expect []string
}

func Test_trimPathSegments(t *testing.T) {
	cases := []trimPathSegmentTestCase{
		{
			Using:  "/",
			Expect: []string{},
		},
		{
			Using:  "/test",
			Expect: []string{"test"},
		},
		{
			Using:  "//test",
			Expect: []string{"test"},
		},
		{
			Using:  "/test/",
			Expect: []string{"test"},
		},
		{
			Using:  "/test/test",
			Expect: []string{"test", "test"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Using, func(t *testing.T) {
			segments := trimPathSegments(tc.Using)
			t.Logf("Expect %#v, calculated %#v", tc.Expect, segments)
			if len(segments) != len(tc.Expect) {
				t.Fatalf("Expected %d segments; got %d", len(tc.Expect), len(segments))
			}
			for i := 0; i < len(tc.Expect); i++ {
				var (
					a = segments[i]
					b = tc.Expect[i]
				)

				if a != b {
					t.Fatalf("Expected segment (%d) to equal %s; got %s", i, b, a)
				}
			}
		})
	}
}
