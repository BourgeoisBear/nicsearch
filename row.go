package main

import (
	"bytes"
	"net/netip"
	"sort"
	"strconv"

	"github.com/BourgeoisBear/range2cidr"
	gerr "github.com/pkg/errors"
)

type TypeKey int

const (
	TkASN TypeKey = iota
	TkIP4
	TkIP6
)

type Row struct {
	Registry []byte
	Cc       []byte
	Type     []byte
	Start    []byte
	Value    []byte
	Date     []byte
	Status   []byte
	RegId    []byte

	// interpreted fields
	ValueInt int
	ASN      uint32
	AsName   []byte
	IpStart  netip.Addr
	IpRange  []netip.Prefix
	TypeInt  TypeKey
}

func (pR *Row) IsType(s ...TypeKey) bool {
	for ix := range s {
		if s[ix] == pR.TypeInt {
			return true
		}
	}
	return false
}

func SortRows(rows []Row) map[TypeKey][]*Row {

	keys := []TypeKey{TkIP4, TkIP6, TkASN}
	var counts [3]uint
	var ret [3][]*Row

	// get counts
	for ix := range rows {
		for ixKey, key := range keys {
			if rows[ix].IsType(key) {
				counts[ixKey] += 1
				break
			}
		}
	}

	// pre-allocate
	for ixKey, _ := range keys {
		ret[ixKey] = make([]*Row, 0, counts[ixKey])
	}

	// add
	for ix := range rows {
		for ixKey, key := range keys {
			if rows[ix].IsType(key) {
				ret[ixKey] = append(ret[ixKey], &rows[ix])
				break
			}
		}
	}

	return map[TypeKey][]*Row{
		TkIP4: ret[0],
		TkIP6: ret[1],
		TkASN: ret[2],
	}
}

func ParseRow(line []byte) (ret Row, err error) {

	sDst := []*[]byte{
		&ret.Registry,
		&ret.Cc,
		&ret.Type,
		&ret.Start,
		&ret.Value,
		&ret.Date,
		&ret.Status,
		&ret.RegId,
	}

	// copy fields
	row := bytes.Split(line, []byte("|"))
	for ix := range row {
		// NOTE: Clone() to avoid cross-referencing the same slice
		val := Clone(bytes.TrimSpace(row[ix]))
		if ix >= len(sDst) {
			break
		}
		*sDst[ix] = val
	}

	// convert type to int
	szType := string(ret.Type)
	switch szType {
	case "ASN":
		ret.TypeInt = TkASN
	case "IPV4":
		ret.TypeInt = TkIP4
	case "IPV6":
		ret.TypeInt = TkIP6
	default:
		err = gerr.Errorf("unexpected row type '%s'", szType)
		return
	}

	// convert value to int
	ret.ValueInt, err = strconv.Atoi(string(ret.Value))
	if err != nil {
		err = gerr.WithMessage(err, "col 5, number expected")
		return
	}

	// convert ASN to uint32
	if ret.IsType(TkASN) {
		var u64 uint64
		u64, err = strconv.ParseUint(string(ret.Start), 10, 32)
		if err != nil {
			return ret, gerr.New("invalid ASN number")
		}
		ret.ASN = uint32(u64)
		return
	}

	// early exit for non-ip records
	is4 := ret.IsType(TkIP4)
	is6 := ret.IsType(TkIP6)
	if !is4 && !is6 {
		return
	}

	// first host address
	ip, e2 := netip.ParseAddr(string(ret.Start))
	if e2 != nil {
		err = gerr.WithMessage(err, "col 4, ip addr expected")
		return
	}
	ret.IpStart = ip

	// validate address family
	if (is4 && !ip.Is4()) || (is6 && !ip.Is6()) {
		err = gerr.New("ip address / label version mismatch")
		return
	}

	// fill IpRange with list of network prefixes
	if is6 {
		ret.IpRange = []netip.Prefix{netip.PrefixFrom(ip, ret.ValueInt)}
		return
	}

	// get base addr as int
	uBase, ok := range2cidr.V4ToUint32(ip)
	if !ok {
		err = gerr.New("failed to convert v4 address to uint32")
		return
	}

	// calculate last host addr, then convert back to netip
	ipLast := range2cidr.Uint32ToV4(uBase + uint32(ret.ValueInt) - 1)

	// create network masks from range
	ret.IpRange, e2 = range2cidr.Deaggregate(ip, ipLast)
	if e2 != nil {
		err = gerr.WithMessage(e2, "ip deaggregation failure")
		return
	}

	return
}

// returns map of reg-ids to row pointers and its corresponding sorted keys
func UniqueRegIds(sRows []Row) (byRegId map[string]*Row, sKeys []string) {

	// map unique reg-ids
	byRegId = make(map[string]*Row)
	for ix, r := range sRows {
		key := string(r.Registry) + string(r.RegId)
		byRegId[key] = &sRows[ix]
	}

	// collect sKeys, sort
	sKeys = make([]string, 0, len(byRegId))
	for k := range byRegId {
		sKeys = append(sKeys, k)
	}
	sort.Strings(sKeys)

	return
}
