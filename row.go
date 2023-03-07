package main

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strconv"

	"github.com/BourgeoisBear/range2cidr"
	gerr "github.com/pkg/errors"
	"go.etcd.io/bbolt"
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
}

func (pR *Row) IsType(s ...string) bool {
	for ix := range s {
		if bytes.Equal(pR.Type, []byte(s[ix])) {
			return true
		}
	}
	return false
}

func SortRows(rows []Row) map[string][]*Row {

	keys := []string{"ipv4", "ipv6", "asn"}
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

	return map[string][]*Row{
		"ipv4": ret[0],
		"ipv6": ret[1],
		"asn":  ret[2],
	}
}

func FindAsName(db *bbolt.DB, nASN uint32) ([]byte, error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// rows
	bktAsName, err := GetBucket(tx, BiAsName.Key())
	if err != nil {
		return nil, err
	}

	var bsASN [4]byte
	binary.BigEndian.PutUint32(bsASN[:], uint32(nASN))
	return bktAsName.Get(bsASN[:]), nil
}

type WalkRawFunc func(k, v []byte) error

func WalkRawRows(db *bbolt.DB, fnWalk WalkRawFunc) error {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// rows
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return err
	}

	return bktRows.ForEach(fnWalk)
}

func (r Row) FindAssociated(db *bbolt.DB) ([]Row, error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// walk keys of id2ix[r.Registry][r.RegId]
	bktIdIx, err := GetBucket(tx, BiId2Ix.Key())
	if err != nil {
		return nil, err
	}

	bktReg, err := GetBucket(bktIdIx, r.Registry)
	if err != nil {
		return nil, err
	}

	bktId := bktReg.Bucket(r.RegId)
	if bktId == nil {
		return nil, nil
	}

	// rows
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return nil, err
	}

	ret := make([]Row, 0)
	err = bktId.ForEach(func(k, v []byte) error {
		if v := bktRows.Get(k); len(v) > 0 {
			row, e2 := ParseRow(v, true)
			if e2 != nil {
				return e2
			}
			ret = append(ret, row)
		}
		return nil
	})
	return ret, err
}

func ParseRow(line []byte, bFillRange bool) (ret Row, err error) {

	row := bytes.Split(line, []byte("|"))

	// NOTE: Clone() to ensure we're not referencing
	// something else's slice
	for ix := range row {
		val := Clone(bytes.TrimSpace(row[ix]))
		switch ix {
		case 0:
			ret.Registry = val
		case 1:
			ret.Cc = val
		case 2:
			ret.Type = val
		case 3:
			ret.Start = val
		case 4:
			ret.Value = val
		case 5:
			ret.Date = val
		case 6:
			ret.Status = val
		case 7:
			ret.RegId = val
		}
	}

	// convert value to int
	nVal, err := strconv.Atoi(string(ret.Value))
	if err != nil {
		err = gerr.WithMessage(err, "col 5, number expected")
		return
	}
	ret.ValueInt = nVal

	// convert ASN to uint32
	if ret.IsType("asn") {
		u64, err := strconv.ParseUint(string(ret.Start), 10, 32)
		if err != nil {
			return ret, gerr.New("invalid ASN number")
		}
		ret.ASN = uint32(u64)
	}

	// early exit for non-ip records
	is4 := ret.IsType("ipv4")
	is6 := ret.IsType("ipv6")
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

	// skip/proceed with CIDR deaggregation
	if !bFillRange {
		return
	}

	// fill IpRange with list of network prefixes
	if is6 {
		ret.IpRange = []netip.Prefix{netip.PrefixFrom(ip, nVal)}
		return
	}

	// get base addr as int
	uBase, ok := range2cidr.V4ToUint32(ip)
	if !ok {
		err = gerr.New("failed to convert v4 address to uint32")
		return
	}

	// calculate last host addr, then convert back to netip
	ipLast := range2cidr.Uint32ToV4(uBase + uint32(nVal) - 1)

	// create network masks from range
	ret.IpRange, e2 = range2cidr.Deaggregate(ip, ipLast)
	if e2 != nil {
		err = gerr.WithMessage(e2, "ip deaggregation failure")
		return
	}

	return
}
