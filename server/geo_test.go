/* ----- ----- ----- ----- */
// geo_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"net/netip"
	"testing"
)

// fakeGeo places a few public addresses.
func fakeGeo(ip netip.Addr) (GeoPlace, bool) {
	places := map[string]GeoPlace{
		"1.34.0.1":    {Country: "TW", Subdivision: "Taipei", City: "Taipei"},
		"61.216.0.1":  {Country: "TW", Subdivision: "Taipei", City: "Taipei"},
		"1.160.0.1":   {Country: "TW", Subdivision: "Kaohsiung", City: "Kaohsiung"},
		"8.8.8.8":     {Country: "US", Subdivision: "California", City: "Mountain View"},
		"9.9.9.9":     {Country: "US", Subdivision: "Ohio", City: "Springfield"},
		"9.9.9.10":    {Country: "US", Subdivision: "Illinois", City: "Springfield"},
		"5.5.5.5":     {Country: "DE", City: "Berlin", CityId: 2950159},
		"5.5.5.6":     {Country: "DE", City: "Berlin (Mitte)", CityId: 2950159},
		"2001:db9::1": {Country: "TW", Subdivision: "Taipei", City: "Taipei"},
	}
	p, ok := places[ip.String()]
	return p, ok
}

func TestCloseLogins(t *testing.T) {
	cases := []struct {
		a, b  string
		close bool
	}{
		{"1.34.0.1:5000", "1.34.0.1:6000", true},      // same IP
		{"127.0.0.1:5000", "192.168.1.20:6000", true}, // both local
		{"[::1]:5000", "10.0.0.3:6000", true},         // both local (IPv6 loopback)
		{"[::ffff:1.34.0.1]:1", "1.34.0.1:2", true},   // IPv4-mapped is the same IP
		{"1.34.0.1:5000", "61.216.0.1:6000", true},    // same city
		{"1.34.0.1:5000", "[2001:db9::1]:6000", true}, // same city, IPv4 and IPv6
		{"5.5.5.5:1", "5.5.5.6:2", true},              // same city id
		{"1.34.0.1:5000", "1.160.0.1:6000", false},    // same country, another city
		{"1.34.0.1:5000", "8.8.8.8:6000", false},      // another country
		{"9.9.9.9:1", "9.9.9.10:2", false},            // same city name, another subdivision
		{"1.34.0.1:5000", "4.4.4.4:6000", false},      // one not found
		{"4.4.4.4:5000", "4.4.4.5:6000", false},       // neither found
		{"127.0.0.1:5000", "1.34.0.1:6000", false},    // local and public
		{"garbage", "garbage", true},                  // unparsable but equal
		{"garbage", "1.34.0.1:1", false},              // unparsable
	}
	for _, c := range cases {
		if got := closeLogins(c.a, c.b, fakeGeo); got != c.close {
			t.Errorf("closeLogins(%s, %s) = %v", c.a, c.b, got)
		}
		if got := closeLogins(c.b, c.a, fakeGeo); got != c.close {
			t.Errorf("closeLogins(%s, %s) = %v (reversed)", c.b, c.a, got)
		}
	}
}

// Without a database only the same IP or two local addresses count as close.
func TestCloseLoginsWithoutDB(t *testing.T) {
	cases := []struct {
		a, b  string
		close bool
	}{
		{"1.34.0.1:5000", "1.34.0.1:6000", true},
		{"127.0.0.1:5000", "192.168.1.20:6000", true},
		{"1.34.0.1:5000", "61.216.0.1:6000", false},
	}
	for _, c := range cases {
		if got := closeLogins(c.a, c.b, nil); got != c.close {
			t.Errorf("closeLogins(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
