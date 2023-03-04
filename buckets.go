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

	"github.com/pkg/errors"
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
	BiRow BucketIx = iota
	BiAsn
	BiV4
	BiV6
	BiId2Ix
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

func (pb *BktFiller) FillFromFile(fname string) error {

	// raw gzipped data
	pF, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer pF.Close()

	// unzipped size (for progress report)
	nLen, err := GetGzipSize(pF)
	if err != nil {
		return err
	}

	// gunzip
	gzr, err := gzip.NewReader(pF)
	if err != nil {
		return err
	}
	defer gzr.Close()

	// add rows to boltdb
	err = pb.FillFromReader(gzr, fname, nLen)
	if err != nil {
		return err
	}

	return nil
}

func (pb *BktFiller) FillFromReader(iRdr io.Reader, name string, ucLen uint32) error {

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

	bsRowIx := make([]byte, 4)

	nBytesRead := 0

	defer func() {
		fmt.Println("")
	}()

	// scan tokens
	bFirstData := true
	pSc := bufio.NewScanner(iRdr)
	var ixFileLine uint32
	for pSc.Scan() {

		ixFileLine += 1
		bsLine := pSc.Bytes()

		// NOTE: +1 for the '\n' omitted by bufio.Scanner
		nBytesRead += len(bsLine) + 1

		// only update progress every 100 rows
		if (ixFileLine%100) == 0 || (nBytesRead >= int(ucLen)) {
			pct := (float32(nBytesRead) / float32(ucLen)) * 100.0
			fmt.Printf("\t\x1b[2K%d/%d (%5.1f%%)\r", nBytesRead, ucLen, pct)
		}

		bsLine = bytes.TrimSpace(bsLine)

		// skip empty
		if len(bsLine) == 0 {
			continue
		}

		// skip comments
		if bytes.HasPrefix(bsLine, []byte("#")) {
			continue
		}

		// skip first data row
		if bFirstData {
			bFirstData = false
			continue
		}

		// skip summaries
		if bytes.HasSuffix(bsLine, []byte("|summary")) {
			continue
		}

		// increment row pk, encode to []byte
		pb.ixRowGlobal += 1
		binary.BigEndian.PutUint32(bsRowIx, pb.ixRowGlobal)

		if err := insertRow(bkt, bsRowIx, bsLine); err != nil {
			err = fmt.Errorf("%s|line %d|\"%s\"|%w", name, ixFileLine, string(bsLine), err)
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

func insertRow(bkt []*bbolt.Bucket, bsRowIx, bsLine []byte) error {

	// only include (allocated|assigned)
	if !bytes.Contains(bsLine, []byte("|assigned|")) &&
		// !bytes.Contains(bsLine, []byte("|reserved|")) &&
		!bytes.Contains(bsLine, []byte("|allocated|")) {
		return nil
	}

	// only include(asn, ipv4, ipv6)
	if !bytes.Contains(bsLine, []byte("|asn|")) &&
		!bytes.Contains(bsLine, []byte("|ipv4|")) &&
		!bytes.Contains(bsLine, []byte("|ipv6|")) {
		return nil
	}

	// parse into values
	oRow, err := ParseRow(bsLine, false)
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
	if oRow.IsType("asn") && (oRow.ValueInt > 0) {

		// insert index for each ASN in range
		asnLast := oRow.ASN + uint32(oRow.ValueInt)
		for asnAdd := oRow.ASN; asnAdd < asnLast; asnAdd += 1 {

			bsASN := Uint32ToBytes(asnAdd)
			err = bkt[BiAsn].Put(bsASN[:], Clone(bsRowIx))
			if err != nil {
				return gerr.WithMessage(err, "put ASN index")
			}
		}

	} else if oRow.IsType("ipv4") || oRow.IsType("ipv6") {

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

func FindByAsn(db *bbolt.DB, nASN uint32) (*Row, error) {

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// ASN index bucket
	bktAsn, err := GetBucket(tx, BiAsn.Key())
	if err != nil {
		return nil, err
	}

	// lookup row index
	bsASN := Uint32ToBytes(nASN)
	rowIx := bktAsn.Get(bsASN[:])
	if rowIx == nil {
		return nil, ENotFound
	}

	// row data bucket
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return nil, err
	}

	// get row data from row index
	bsRow := bktRows.Get(rowIx)
	if len(bsRow) == 0 {
		return nil, errors.New("db err: missing row data to index")
	}

	// parse row data into struct
	ret, err := ParseRow(bsRow, false)
	return &ret, err
}

func FindByIp(db *bbolt.DB, as netip.Addr) (*Row, error) {

	if !as.IsValid() {
		return nil, EInvalidIpAddress
	}

	// start transaction
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// fetch buckets
	bktRows, err := GetBucket(tx, BiRow.Key())
	if err != nil {
		return nil, err
	}

	ipix := BiV4
	if as.Is6() {
		ipix = BiV6
	}
	bktIp, err := GetBucket(tx, ipix.Key())
	if err != nil {
		return nil, err
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
			return nil, ENotFound
		}
		tryReSeek = false
	}

RESEEK:

	// get row data from row index
	bsRow := bktRows.Get(v)
	if len(bsRow) == 0 {
		return nil, ENotFound
	}

	// parse row data into struct
	ret, err := ParseRow(bsRow, true)
	if err != nil {
		return &ret, err
	}

	// check for range membership
	for j := range ret.IpRange {
		if ret.IpRange[j].Contains(as) {
			return &ret, nil
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

	return nil, ENotFound
}
