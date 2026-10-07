/* ----- ----- ----- ----- */
// geo.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"net"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Where a login comes from, for the duplicate-login rule (ONLINE-PLAY 8.14, 8.24): an account
// logging in while another live connection of it exists replaces that connection when both come
// from the same place (closeLogins), and is refused otherwise. The place of a public address is
// looked up in the free DB-IP Lite City database (mmdb, CC BY 4.0: "IP Geolocation by DB-IP",
// https://db-ip.com), whose path is CHESS_GEOIP_DB; the database is not in the repo (download it
// with ai-agent-taki/custom_tools/chinese-chess-server-geoip).

// GeoPlace is where an address is: the country (ISO code), the first subdivision and the city
// (English names), and the city's GeoNames id when the database has one (DB-IP Lite has none).
type GeoPlace struct {
	Country     string
	Subdivision string
	City        string
	CityId      uint
}

// geoLookup finds the place of a public address; false when it is not known.
type geoLookup func(ip netip.Addr) (GeoPlace, bool)

// samePlace is whether two places are the same city: the same GeoNames id when both have one,
// otherwise the same country, city and (when both have one) subdivision.
func samePlace(a, b GeoPlace) bool {
	if a.CityId != 0 && b.CityId != 0 {
		return a.CityId == b.CityId
	}
	if a.Country == "" || a.City == "" {
		return false
	}
	if a.Subdivision != "" && b.Subdivision != "" && a.Subdivision != b.Subdivision {
		return false
	}
	return a.Country == b.Country && a.City == b.City
}

// localAddr is whether ip is a loopback, private or link-local address (no place to look up).
func localAddr(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// hostOf is the address of a remote address string ("host:port", or a bare host).
func hostOf(remote string) string {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}

// closeLogins is whether two logins of an account, from the remote addresses a and b, are close
// enough for the newer one to replace the older: the same address; both local (loopback / private);
// or both public and found in the same city by lookup. Without a lookup (nil: no database) only the
// first two count.
func closeLogins(a, b string, lookup geoLookup) bool {
	ha, hb := hostOf(a), hostOf(b)
	ipa, erra := netip.ParseAddr(ha)
	ipb, errb := netip.ParseAddr(hb)
	if erra != nil || errb != nil {
		return ha == hb
	}
	ipa, ipb = ipa.Unmap(), ipb.Unmap()
	if ipa == ipb {
		return true
	}
	if localAddr(ipa) && localAddr(ipb) {
		return true
	}
	if lookup == nil || localAddr(ipa) || localAddr(ipb) {
		return false
	}
	pa, oka := lookup(ipa)
	pb, okb := lookup(ipb)
	return oka && okb && samePlace(pa, pb)
}

// GeoDB is an open DB-IP Lite City (or GeoLite2 City) database.
type GeoDB struct {
	reader *maxminddb.Reader
}

// OpenGeoDB opens the mmdb database at path.
func OpenGeoDB(path string) (*GeoDB, error) {
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &GeoDB{reader: r}, nil
}

// Close closes the database.
func (g *GeoDB) Close() error { return g.reader.Close() }

// geoRecord is the part of a city record GeoDB reads.
type geoRecord struct {
	City struct {
		GeonameId uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

// Lookup is the place of ip; false when the database does not know it.
func (g *GeoDB) Lookup(ip netip.Addr) (GeoPlace, bool) {
	var rec geoRecord
	res := g.reader.Lookup(ip)
	if !res.Found() {
		return GeoPlace{}, false
	}
	if err := res.Decode(&rec); err != nil {
		return GeoPlace{}, false
	}
	p := GeoPlace{Country: rec.Country.IsoCode, City: rec.City.Names["en"], CityId: rec.City.GeonameId}
	if len(rec.Subdivisions) > 0 {
		p.Subdivision = rec.Subdivisions[0].Names["en"]
	}
	return p, p.Country != "" && (p.City != "" || p.CityId != 0)
}
