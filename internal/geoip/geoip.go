package geoip

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
)

type node struct {
	child   [2]*node
	country string
}

// Database is a small, offline CIDR-to-country lookup table.
// The file format is CSV with: CIDR,country_code.
type Database struct {
	root4 *node
	root6 *node
}

func Open(path string) (*Database, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open geoip database: %w", err)
	}
	defer f.Close()
	return LoadCSV(f)
}

func LoadCSV(r io.Reader) (*Database, error) {
	db := &Database{root4: &node{}, root6: &node{}}
	reader := csv.NewReader(bufio.NewReader(r))
	reader.FieldsPerRecord = -1
	line := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("read geoip database line %d: %w", line, err)
		}
		if len(record) == 0 || strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}
		if len(record) < 2 {
			return nil, fmt.Errorf("invalid geoip database line %d: expected CIDR,country", line)
		}
		cidr := strings.TrimSpace(record[0])
		country := strings.ToUpper(strings.TrimSpace(record[1]))
		if cidr == "" || country == "" {
			return nil, fmt.Errorf("invalid geoip database line %d: empty CIDR or country", line)
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid geoip CIDR on line %d: %w", line, err)
		}
		root := db.root6
		if prefix.Addr().Is4() {
			root = db.root4
		}
		insert(root, prefix, country)
	}
	return db, nil
}

func insert(root *node, prefix netip.Prefix, country string) {
	bits := prefix.Bits()
	addr := prefix.Addr()
	var bytes []byte
	if addr.Is4() {
		a := addr.As4()
		bytes = a[:]
	} else {
		a := addr.As16()
		bytes = a[:]
	}
	n := root
	for i := 0; i < bits; i++ {
		bit := (bytes[i/8] >> uint(7-i%8)) & 1
		if n.child[bit] == nil {
			n.child[bit] = &node{}
		}
		n = n.child[bit]
	}
	n.country = country
}

func (db *Database) Lookup(addr netip.Addr) (string, bool) {
	if db == nil || !addr.IsValid() {
		return "", false
	}
	root := db.root6
	var bytes []byte
	if addr.Is4() {
		root = db.root4
		a := addr.As4()
		bytes = a[:]
	} else {
		a := addr.As16()
		bytes = a[:]
	}
	var country string
	n := root
	for i := 0; i < len(bytes)*8 && n != nil; i++ {
		if n.country != "" {
			country = n.country
		}
		bit := (bytes[i/8] >> uint(7-i%8)) & 1
		n = n.child[bit]
	}
	if n != nil && n.country != "" {
		country = n.country
	}
	return country, country != ""
}
