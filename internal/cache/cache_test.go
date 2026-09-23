package cache

import (
	"strconv"
	"testing"

	"github.com/UsmanXTech/shorty/internal/links"
)

func TestCacheSetGetDelete(t *testing.T) {
	c := New()
	link := links.Link{ID: 1, Slug: "abc", URL: "https://example.com"}
	if _, ok := c.Get("abc"); ok {
		t.Fatal("expected cache miss")
	}
	c.Set(link)
	got, ok := c.Get("abc")
	if !ok || got.ID != link.ID || got.URL != link.URL {
		t.Fatalf("unexpected cached link: %+v, ok=%v", got, ok)
	}
	c.Delete("abc")
	if _, ok := c.Get("abc"); ok {
		t.Fatal("expected cache miss after delete")
	}
}

func TestCacheClear(t *testing.T) {
	c := New()
	c.Set(links.Link{Slug: "abc"})
	c.Set(links.Link{Slug: "def"})
	c.Clear()
	if _, ok := c.Get("abc"); ok {
		t.Fatal("expected cache to be empty")
	}
	if _, ok := c.Get("def"); ok {
		t.Fatal("expected cache to be empty")
	}
}

func TestCacheBounded(t *testing.T) {
	c := New()
	for i := 0; i < 10500; i++ {
		c.Set(links.Link{Slug: "slug-" + strconv.Itoa(i), URL: "https://example.com"})
	}
	if n := c.Len(); n > 10000 {
		t.Fatalf("cache must stay bounded, has %d entries", n)
	}
	if _, ok := c.Get("slug-10499"); !ok {
		t.Fatal("most recently set entry should be present")
	}
}
