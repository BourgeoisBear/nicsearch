package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strconv"

	gerr "github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type EFind int

const (
	ENotFound EFind = iota
	EInvalidIpAddress
	EInvalidQuery
)

func (v EFind) Error() string {
	switch v {
	case ENotFound:
		return "no matches found"
	case EInvalidIpAddress:
		return "invalid IP address"
	case EInvalidQuery:
		return "invalid query"
	}
	return "invalid EFind value"
}

func Clone(in []byte) []byte {
	if in != nil {
		out := make([]byte, len(in))
		copy(out, in)
		return out
	}
	return nil
}

func Uint32ToBytes(in uint32) [4]byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], in)
	return out
}

type BucketIx int

const (
	BiRow    BucketIx = iota // map[rowIndex]rowData
	BiAsn                    // map[uint32 ASN]rowIndex
	BiV4                     // map[v4 address]rowIndex
	BiV6                     // map[v6 address]rowIndex
	BiId2Ix                  // map[registry][regId][rowIndex]interface{}
	BiAsName                 // map[uint32 ASN]ASName
	BiMAX
)

func (k BucketIx) Key() []byte {
	switch k {
	case BiRow:
		return []byte("rows")
	case BiAsn:
		return []byte("asn")
	case BiV4:
		return []byte("v4")
	case BiV6:
		return []byte("v6")
	case BiId2Ix:
		return []byte("id2ix")
	case BiAsName:
		return []byte("asname")
	}
	return []byte{}
}

type IHasBucket interface {
	Bucket([]byte) *bbolt.Bucket
}

func GetBucket(ib IHasBucket, bsKey []byte) (*bbolt.Bucket, error) {
	bkt := ib.Bucket(bsKey)
	if bkt == nil {
		return nil, fmt.Errorf("BUCKET %s: not found", string(bsKey))
	}
	return bkt, nil
}

type BktFiller struct {
	db          *bbolt.DB
	ixRowGlobal uint32

	bSkipFirstDataRow bool
}

func CreateBktFiller(db *bbolt.DB) (*BktFiller, error) {

	// start transaction
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// create buckets
	for ix := BucketIx(0); ix < BiMAX; ix++ {
		if _, err = tx.CreateBucket(ix.Key()); err != nil {
			return nil, err
		}
	}

	return &BktFiller{db: db}, tx.Commit()
}

func GetGzipSize(pF *os.File) (uint32, error) {

	// 4 bytes from end
	_, err := pF.Seek(-4, 2)
	if err != nil {
		return 0, err
	}

	// read
	bs := make([]byte, 4)
	_, err = pF.Read(bs)
	if err != nil && err != io.EOF {
		return 0, err
	}

	// decode
	nLen := binary.LittleEndian.Uint32(bs)

	// reset to beginning
	_, err = pF.Seek(0, 0)
	return nLen, err
}

type GzReadFunc func(
	bkt []*bbolt.Bucket,
	bsLine []byte,
) error

func (pb *BktFiller) GzRead(fname string, fnRead GzReadFunc) error {

	// raw gzipped data
	pF, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer pF.Close()

	// unzipped size (for progress report)
	ucLen32, err := GetGzipSize(pF)
	if err != nil {
		return err
	}
	ucLen := uint64(ucLen32)

	// gunzip
	gzr, err := gzip.NewReader(pF)
	if err != nil {
		return err
	}
	defer gzr.Close()

	// start transaction
	tx, eTx := pb.db.Begin(true)
	if eTx != nil {
		return eTx
	}
	defer tx.Rollback()

	// get buckets
	bkt := make([]*bbolt.Bucket, BiMAX)
	for ix := BucketIx(0); ix < BiMAX; ix++ {
		bkt[ix] = tx.Bucket(ix.Key())
	}

	defer func() {
		fmt.Fprintln(os.Stderr, "")
	}()

	// scan tokens
	pb.bSkipFirstDataRow = true
	pSc := bufio.NewScanner(gzr)
	var ixFileLine, nBytesRead uint64
	for pSc.Scan() {

		bsLine := pSc.Bytes()
		ixFileLine += 1
		nBytesRead += uint64(len(bsLine) + 1) // +1 for '\n'

		// only update progress every 100 rows
		if (ixFileLine%100) == 0 || (nBytesRead >= ucLen) {
			pct := (float64(nBytesRead) / float64(ucLen)) * 100.0
			fmt.Fprintf(
				os.Stderr,
				"\x1b[2K(%5.1f%%) %9d/%-9d bytes\r",
				pct, nBytesRead, ucLen,
			)
		}

		// skip empty
		bsLine = bytes.TrimSpace(bsLine)
		if len(bsLine) == 0 {
			continue
		}

		err := fnRead(bkt, bsLine)
		if err != nil {
			err = fmt.Errorf("%s|line %d|\"%s\"|%w", fname, ixFileLine, string(bsLine), err)
			fmt.Fprintln(os.Stderr, err)
		}
	}

	// check for scanner errs
	if err := pSc.Err(); err != nil {
		return err
	}

	// commit the transaction
	return tx.Commit()
}

var g_rxSplitAsn *regexp.Regexp = regexp.MustCompile(`^\s*([0-9]+)\s+(.+)\s*$`)

func (pb *BktFiller) scanlnAsName(bkt []*bbolt.Bucket, bsLine []byte) error {

	// get buckets
	bktAsName := bkt[BiAsName]

	// extract ASN & description from line
	sMatch := g_rxSplitAsn.FindSubmatch(bsLine)
	if len(sMatch) < 3 {
		return nil
	}

	nASN, err := strconv.ParseUint(string(sMatch[1]), 10, 32)
	if err != nil {
		return nil
	}

	// add to kv store
	var bsASN [4]byte
	binary.BigEndian.PutUint32(bsASN[:], uint32(nASN))
	bsName := bytes.ToUpper(sMatch[2])
	return bktAsName.Put(bsASN[:], bsName)
}

func (pb *BktFiller) scanlnDelegation(bkt []*bbolt.Bucket, bsLine []byte) error {

	// skip comments
	if bytes.HasPrefix(bsLine, []byte("#")) {
		return nil
	}

	// skip first data row
	if pb.bSkipFirstDataRow {
		pb.bSkipFirstDataRow = false
		return nil
	}

	// skip summaries
	if bytes.HasSuffix(bsLine, []byte("|summary")) {
		return nil
	}

	// increment row pk, encode to []byte
	pb.ixRowGlobal += 1
	var bsRowIx [4]byte
	binary.BigEndian.PutUint32(bsRowIx[:], pb.ixRowGlobal)
	return insertRow(bkt, bsRowIx[:], bsLine)
}

func insertRow(bkt []*bbolt.Bucket, bsRowIx, bsLine []byte) error {

	bsLine = bytes.ToUpper(bsLine)

	// only include (allocated|assigned)
	if !bytes.Contains(bsLine, []byte("|ASSIGNED|")) &&
		// !bytes.Contains(bsLine, []byte("|RESERVED|")) &&
		!bytes.Contains(bsLine, []byte("|ALLOCATED|")) {
		return nil
	}

	// only include(asn, ipv4, ipv6)
	if !bytes.Contains(bsLine, []byte("|ASN|")) &&
		!bytes.Contains(bsLine, []byte("|IPV4|")) &&
		!bytes.Contains(bsLine, []byte("|IPV6|")) {
		return nil
	}

	// parse into values
	oRow, err := ParseRow(bsLine)
	if err != nil {
		return gerr.WithMessage(err, "parse row")
	}

	// insert row
	err = bkt[BiRow].Put(bsRowIx, Clone(bsLine))
	if err != nil {
		return gerr.WithMessage(err, "put row")
	}

	// RegId sub-buckets
	if len(oRow.RegId) > 0 && len(oRow.Registry) > 0 {
		bktIdReg, err := bkt[BiId2Ix].CreateBucketIfNotExists(oRow.Registry)
		if err != nil {
			return gerr.WithMessage(err, "bkt id2ix:reg")
		}
		id2ix, err := bktIdReg.CreateBucketIfNotExists(oRow.RegId)
		if err != nil {
			return gerr.WithMessage(err, "bkt id2ix:regid")
		}
		if err = id2ix.Put(bsRowIx, nil); err != nil {
			return gerr.WithMessage(err, "put id2ix:rowix")
		}
	}

	// update asn, ipv4, ipv6 indices
	if oRow.IsType(TkASN) && (oRow.ValueInt > 0) {

		// insert index for each ASN in range
		asnLast := oRow.ASN + uint32(oRow.ValueInt)
		for asnAdd := oRow.ASN; asnAdd < asnLast; asnAdd += 1 {

			bsASN := Uint32ToBytes(asnAdd)
			err = bkt[BiAsn].Put(bsASN[:], Clone(bsRowIx))
			if err != nil {
				return gerr.WithMessage(err, "put ASN index")
			}
		}

	} else if oRow.IsType(TkIP4, TkIP6) {

		if !oRow.IpStart.IsValid() {
			return gerr.New("invalid ip")
		}

		if oRow.IpStart.Is4() {
			v := oRow.IpStart.As4()
			err = bkt[BiV4].Put(v[:], Clone(bsRowIx))
		} else {
			v := oRow.IpStart.As16()
			err = bkt[BiV6].Put(v[:], Clone(bsRowIx))
		}

		if err != nil {
			return gerr.WithMessage(err, "put ip index")
		}
	}

	return nil
}

type RowIndex []byte

func GetRow(tx *bbolt.Tx, rowIx RowIndex) (Row, error) {

	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return Row{}, err
	}

	bsRow := bktRows.Get(rowIx)
	if len(bsRow) == 0 {
		return Row{}, ENotFound
	}

	return ParseRow(bsRow)
}

func AsnToName(db *bbolt.DB, nASN uint32) ([]byte, error) {
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var bsASN [4]byte
	binary.BigEndian.PutUint32(bsASN[:], uint32(nASN))
	return AsnToNameTx(tx, bsASN[:])
}

func AsnToNameTx(tx *bbolt.Tx, asn []byte) ([]byte, error) {
	bktAsName, err := GetBucket(tx, BiAsName.Key())
	if err != nil {
		return nil, err
	}
	bsName := bktAsName.Get(asn)
	if len(bsName) > 0 {
		return Clone(bsName), nil
	}
	return nil, ENotFound
}

func AsnToRow(db *bbolt.DB, nASN uint32) (Row, error) {
	tx, err := db.Begin(false)
	if err != nil {
		return Row{}, err
	}
	defer tx.Rollback()
	bsASN := Uint32ToBytes(nASN)
	return AsnToRowTx(tx, bsASN[:])
}

func AsnToRowTx(tx *bbolt.Tx, bsASN []byte) (Row, error) {

	// ASN index bucket
	bktAsn, err := GetBucket(tx, BiAsn.Key())
	if err != nil {
		return Row{}, err
	}

	// lookup row index
	rowIx := bktAsn.Get(bsASN)
	if rowIx == nil {
		return Row{}, ENotFound
	}

	// get row data from row index
	return GetRow(tx, rowIx)
}

func IpToRow(db *bbolt.DB, ip netip.Addr) (Row, error) {
	tx, err := db.Begin(false)
	if err != nil {
		return Row{}, err
	}
	defer tx.Rollback()
	return IpToRowTx(tx, ip)
}

func IpToRowTx(tx *bbolt.Tx, ip netip.Addr) (Row, error) {

	if !ip.IsValid() {
		return Row{}, EInvalidIpAddress
	}

	// fetch buckets
	ipix := BiV4
	if ip.Is6() {
		ipix = BiV6
	}
	bktIp, err := GetBucket(tx, ipix.Key())
	if err != nil {
		return Row{}, err
	}

	// lookup row index by ip
	binIpAddr := ip.AsSlice()
	curBktIp := bktIp.Cursor()
	k, v := curBktIp.Seek(binIpAddr)

	tryReSeek := true

	// case: not found, or in last network range
	if k == nil {
		// fetch row index of last network range
		k, v = curBktIp.Last()
		if k == nil {
			return Row{}, ENotFound
		}
		tryReSeek = false
	}

RESEEK:

	// get row data from row index
	ret, err := GetRow(tx, v)
	if err != nil {
		return ret, err
	}

	// check for range membership
	for j := range ret.IpRange {
		if ret.IpRange[j].Contains(ip) {
			return ret, nil
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

	return Row{}, ENotFound
}

func FindAssociated(
	db *bbolt.DB, bsRegistry, bsRegId []byte,
) ([]Row, error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// walk keys of id2ix[bsRegistry][bsRegId]
	bktIdIx, err := GetBucket(tx, BiId2Ix.Key())
	if err != nil {
		return nil, err
	}

	bktReg, err := GetBucket(bktIdIx, bsRegistry)
	if err != nil {
		return nil, err
	}

	bktId := bktReg.Bucket(bsRegId)
	if bktId == nil {
		return nil, nil
	}

	// rows
	ret := make([]Row, 0)
	err = bktId.ForEach(func(bsRowIx, _ []byte) error {
		row, e2 := GetRow(tx, bsRowIx)
		if e2 == nil {
			ret = append(ret, row)
		}
		return e2
	})
	return ret, err
}

func NameRegexToASNs(db *bbolt.DB, rxName string) ([]Row, error) {

	rx, err := regexp.Compile(`(?i)` + rxName)
	if err != nil {
		return nil, err
	}

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// ASN name bucket
	bktAsName, err := GetBucket(tx, BiAsName.Key())
	if err != nil {
		return nil, err
	}

	// regex match name on all buckets
	sAsn := make([]Row, 0, 100)
	err = bktAsName.ForEach(func(bsASN, bsAsName []byte) error {

		// regex match AS Name
		if !rx.Match(bsAsName) {
			return nil
		}

		// get data row for ASN
		row, err := AsnToRowTx(tx, bsASN)
		if err != nil {
			if err == ENotFound {
				return nil
			} else {
				return err
			}
		}

		// only return first row for ranges
		nASN := binary.BigEndian.Uint32(bsASN)
		if row.ASN == nASN {
			row.AsName = Clone(bsAsName)
			sAsn = append(sAsn, row)
		}
		return nil
	})

	return sAsn, err
}

type WalkRawFunc func(rowIx, rowData []byte) error

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
