package geoip

import (
	"strings"
	"testing"

	"net/netip"
)

func TestLoadCSVAndLookupUsesLongestPrefix(t *testing.T) {
	db, err := LoadCSV(strings.NewReader(`# local test data
0.0.0.0/0,ZZ
8.0.0.0/8,US
8.8.8.0/24,US
2001:db8::/32,ZZ
2001:db8:1::/48,PK
`))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		ip      string
		country string
	}{
		{"1.1.1.1", "ZZ"},
		{"8.8.8.8", "US"},
		{"2001:db8:1::10", "PK"},
		{"2001:db8:2::10", "ZZ"},
	}
	for _, tc := range cases {
		got, ok := db.Lookup(netip.MustParseAddr(tc.ip))
		if !ok || got != tc.country {
			t.Fatalf("Lookup(%s) = %q, %v; want %q, true", tc.ip, got, ok, tc.country)
		}
	}
}

func TestLoadCSVRejectsInvalidRows(t *testing.T) {
	if _, err := LoadCSV(strings.NewReader("not-a-cidr,PK\n")); err == nil {
		t.Fatal("expected invalid CIDR error")
	}
	if _, err := LoadCSV(strings.NewReader("1.2.3.0/24\n")); err == nil {
		t.Fatal("expected missing country error")
	}
}
