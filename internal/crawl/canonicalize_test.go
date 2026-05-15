package crawl

import "testing"

func TestCanonicalize_TableDriven(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"HTTPS://Example.COM/foo/", "https://example.com/foo"},
		{"http://example.com:80/", "http://example.com/"},
		{"https://example.com:443/foo", "https://example.com/foo"},
		{"https://example.com/x#anchor", "https://example.com/x"},
		{"https://example.com/x?utm_source=a&b=2&a=1", "https://example.com/x?a=1&b=2"},
		{"https://example.com/x?gclid=xyz", "https://example.com/x"},
		{"https://example.com/a/../b", "https://example.com/b"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com/a/b/", "https://example.com/a/b"},
	}
	for _, c := range cases {
		got, err := Canonicalize(c.in)
		if err != nil {
			t.Errorf("err on %q: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s -> %s, want %s", c.in, got, c.want)
		}
	}
}

func TestCanonicalize_Idempotent(t *testing.T) {
	in := "https://example.com/foo?bar=1"
	one, _ := Canonicalize(in)
	two, _ := Canonicalize(one)
	if one != two {
		t.Fatalf("not idempotent: %q vs %q", one, two)
	}
}

func TestSameHost(t *testing.T) {
	if !SameHost("https://example.com/a", "https://example.com/b") {
		t.Fatal("expected same")
	}
	if SameHost("https://example.com", "https://other.com") {
		t.Fatal("expected different")
	}
}
