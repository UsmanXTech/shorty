package utm

import "testing"

func TestBuildPreservesQueryAndEncodesUTM(t *testing.T) {
	got, err := Build("https://example.com/path?existing=yes", Params{
		Source:   "newsletter",
		Medium:   "email",
		Campaign: "fall launch",
		Term:     "short links",
		Content:  "hero button",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/path?existing=yes&utm_campaign=fall+launch&utm_content=hero+button&utm_medium=email&utm_source=newsletter&utm_term=short+links"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuildIgnoresEmptyUTMValues(t *testing.T) {
	got, err := Build("https://example.com", Params{Source: "social"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com?utm_source=social" {
		t.Fatalf("Build() = %q", got)
	}
}

func TestBuildRejectsInvalidURL(t *testing.T) {
	for _, raw := range []string{"example.com", "ftp://example.com/file", "https:///missing-host"} {
		if _, err := Build(raw, Params{}); err == nil {
			t.Fatalf("Build(%q) expected error", raw)
		}
	}
}
