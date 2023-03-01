package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/BourgeoisBear/range2cidr"
	gerr "github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// ls *.go *.gz | entr -c go run nicsearch.go
// Ctrl-w N (copy)
// Ctrl-w "+ (paste)
func main() {

	var E error
	defer func() {
		if E != nil {
			fmt.Println(E)
			os.Exit(1)
		}
	}()

	DBDIR := "./db"

	// flags
	var bReIndex, bDownload bool
	flag.BoolVar(&bReIndex, "reindex", false, "rebuild index into RIR database")
	flag.BoolVar(&bDownload, "download", false, "download RIR databases")
	flag.Parse()

	// download
	if bDownload {

		bReIndex = true

		// download delegations from each RIR
		mRIR := GetRIRs()
		for key := range mRIR {
			E = DownloadRIRD(mRIR[key], DBDIR, DBDIR)
			if E != nil {
				return
			}
		}
	}

	// bolt
	boltDbFname := DBDIR + "/nicsearch.db"

	if bReIndex {
		os.Remove(boltDbFname)
	}

	// TODO: ask for update-db if not exist
	db, E := bbolt.Open(boltDbFname, 0600, nil)
	if E != nil {
		return
	}
	defer db.Close()

	// TODO: capture & report RIR list dates & versions?

	if bReIndex {

		// create buckets
		pBkt, err := CreateBktFiller(db)
		if err != nil {
			E = err
			return
		}

		// TODO: $HOME/.cache/APPNAME/

		// fill from sources
		mRIR := GetRIRs()
		for key := range mRIR {
			fname := DBDIR + "/" + mRIR[key].Filename + ".txt.gz"
			fmt.Printf("\x1b[1mINDEXING:\x1b[0m %s\n", fname)
			E = pBkt.FillFromFile(fname)
			if E != nil {
				return
			}
		}

		return
	}

	// TODO: stdin commands instead of flags
	as := netip.MustParseAddr("23.239.224.0")
	as = netip.MustParseAddr("13.116.0.21") // ripe
	// as = netip.MustParseAddr("14.1.96.7")
	// as = netip.MustParseAddr("45.4.68.1")
	// oAsn, found := LookupASN(lines, "7903")
	// as = netip.MustParseAddr("192.91.139.50")
	fmt.Println(as)

	row, found, E := FindByIp(db, as)
	if E != nil {
		return
	}

	// row, found, E = FindByAsn(db, "arin", "7903")
	// if E != nil {
	// 	return
	// }

	if found {
		sAssoc, e2 := row.FindAssociated(db)
		if e2 != nil {
			E = e2
			return
		}
		for ix := range sAssoc {
			bsPretty := sAssoc[ix].Pretty()
			os.Stdout.Write(bsPretty)
			os.Stdout.Write([]byte("\n"))
		}
	} else {
		fmt.Println("NOT FOUND")
	}

	return
}

func (r Row) FindAssociated(db *bbolt.DB) ([]Row, error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// fetch buckets
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

func FindByAsn(db *bbolt.DB, registry, asn string) (ret Row, found bool, err error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return
	}
	defer tx.Rollback()

	// ASN index bucket
	bktAsn, err := GetBucket(tx, BiAsn.Key())
	if err != nil {
		return
	}

	// lookup row index
	rowIx := bktAsn.Get([]byte(registry + "|" + asn))
	if rowIx == nil {
		return
	}

	// row data bucket
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return
	}

	// get row data from row index
	bsRow := bktRows.Get(rowIx)
	if len(bsRow) == 0 {
		return
	}

	// parse row data into struct
	found = true
	ret, err = ParseRow(bsRow, false)
	return
}

func FindByIp(db *bbolt.DB, as netip.Addr) (ret Row, found bool, err error) {

	if !as.IsValid() {
		return
	}

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return
	}
	defer tx.Rollback()

	// fetch buckets
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return
	}

	ipix := BiV4
	if as.Is6() {
		ipix = BiV6
	}
	bktIp, err := GetBucket(tx, ipix.Key())
	if err != nil {
		return
	}

	// lookup row index by ip
	binIpAddr := as.AsSlice()
	curBktIp := bktIp.Cursor()
	k, v := curBktIp.Seek(binIpAddr)

	tryReSeek := true

	// case: not found, or in last network range
	if k == nil {
		// fetch row index of last network range
		k, v = curBktIp.Last()
		if k == nil {
			return
		}
		tryReSeek = false
	}

RESEEK:

	// get row data from row index
	bsRow := bktRows.Get(v)
	if len(bsRow) == 0 {
		return
	}

	// parse row data into struct
	ret, err = ParseRow(bsRow, true)
	if err != nil {
		return
	}

	// check for range membership
	for j := range ret.IpRange {
		if ret.IpRange[j].Contains(as) {
			found = true
			return
		}
	}

	// backtrack once
	if tryReSeek {
		tryReSeek = false
		k, v = curBktIp.Prev()
		if k != nil {
			goto RESEEK
		}
	}

	return
}

type Row struct {
	Registry []byte
	Cc       []byte
	Type     []byte
	Start    []byte
	Value    []byte
	Date     []byte
	Status   []byte
	RegId    []byte

	ValueInt int
	IpStart  netip.Addr
	IpRange  []netip.Prefix
}

func (pR *Row) AsnKey() []byte {
	return bytes.Join([][]byte{pR.Registry, pR.Start}, []byte("|"))
}

func (pR *Row) IsType(s ...string) bool {
	for ix := range s {
		if bytes.Equal(pR.Type, []byte(s[ix])) {
			return true
		}
	}
	return false
}

func (pR *Row) Raw() []byte {
	s := [][]byte{
		pR.Registry,
		pR.Cc,
		pR.Type,
		pR.Start,
		pR.Value,
		pR.Date,
		pR.Status,
		pR.RegId,
	}
	return bytes.Join(s, []byte("|"))
}

func (pR *Row) Pretty() []byte {

	s := make([][]byte, 0, 7)
	s = append(s, pR.Registry, pR.Cc, pR.Type)

	if pR.IsType("asn") {

		s = append(s, pR.Start, pR.Value)

	} else {

		// TODO: print row multiple times for each subnet
		if len(pR.IpRange) > 0 {
			sRng := make([]string, 0, len(pR.IpRange))
			for _, r := range pR.IpRange {
				if r.IsValid() {
					sRng = append(sRng, r.String())
				}
			}
			if len(sRng) > 0 {
				s = append(s, []byte(strings.Join(sRng, ",")))
			}
		}
	}

	s = append(s, pR.Date, pR.Status) //, pR.RegId)

	// TODO: tabularize in TTY mode
	return bytes.Join(s, []byte("|"))
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

	nVal, err := strconv.Atoi(string(ret.Value))
	if err != nil {
		err = gerr.WithMessage(err, "col 5, number expected")
		return
	}
	ret.ValueInt = nVal

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
