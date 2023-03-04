package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Modes struct {
	Color  bool
	Pretty bool
}

func (m *Modes) UpdateFromCmd(cmd string) bool {
	switch cmd {
	case "pretty":
		m.Pretty = true
	case "nopretty":
		m.Pretty = false
	default:
		return false
	}
	return true
}

// ls *.go *.gz | entr -c go run nicsearch.go
// Ctrl-w N (copy)
// Ctrl-w "+ (paste)
func main() {

	DBDIR := "./db"

	var mode Modes
	bIsTty := false
	if isatty.IsTerminal(os.Stdout.Fd()) {
		bIsTty = true
	}

	// flags
	var bReIndex, bDownload bool
	flag.BoolVar(&bReIndex, "reindex", false, "rebuild index into RIR database")
	flag.BoolVar(&bDownload, "download", false, "download RIR databases")
	flag.BoolVar(&mode.Color, "color", bIsTty, "force color output on/off")
	flag.BoolVar(&mode.Pretty, "pretty", bIsTty, "force pretty print on/off")
	flag.Parse()

	var E error
	defer func() {
		if E != nil {
			mode.printErr(E)
			os.Exit(1)
		}
	}()

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

	/*
		TODO:
			- reindex if db doesn't exist
			- download if RIR data doesn't exist
				- notify "first run, downloading"
			- configurable directory (default to .cache/APPNAME)
	*/

	db, E := bbolt.Open(boltDbFname, 0600, nil)
	if E != nil {
		return
	}
	defer db.Close()

	if bReIndex {

		// create buckets
		pBkt, err := CreateBktFiller(db)
		if err != nil {
			E = err
			return
		}

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

	sCmds := flag.Args()
	if len(sCmds) == 0 {

		// stdin command mode
		pSc := bufio.NewScanner(os.Stdin)
		for pSc.Scan() {
			if err := mode.doREPL(db, pSc.Text()); err != nil {
				mode.printErr(err)
			}
		}
		// check for scanner errs
		if E = pSc.Err(); E != nil {
			return
		}

	} else {

		// args command mode
		for ix := range sCmds {

			// abort on first error in args mode
			if err := mode.doREPL(db, sCmds[ix]); err != nil {
				E = err
				return
			}
		}
	}

	/*
		23.239.224.0
		13.116.0.21

		146978
		14061 // DIGITALOCEAN
		328499
		7903
	*/

	/*
		TODO:
			- ASN name search (API)

					https://ftp.arin.net/pub/stats/ripencc/delegated-ripencc-latest
					https://ftp.ripe.net/ripe/asnames/asn.txt
					https://ftp.arin.net/info/asn.txt
					https://stat.ripe.net/data/as-overview/data.json?resource=AS14061

					https://stat.ripe.net/docs/02.data-api/abuse-contact-finder.html
					https://securitytrails.com/blog/asn-lookup
					https://bgp.potaroo.net/cidr/autnums.html

			- unit tests

			- RIPE ASNs for DO

					ripencc|US|asn|200130|1|20131001|allocated|7f14c7cb-6fba-4d73-ade6-8ab7cd032bc0
					ripencc|US|asn|201229|
					ripencc|US|asn|202018|
					ripencc|US|asn|202109|
	*/

	return
}

func (m *Modes) printErr(err error) {
	if err == nil {
		return
	}
	if m.Color {
		fmt.Fprintln(os.Stderr, "\x1b[91;1m"+"error"+"\x1b[0m: "+err.Error())
	} else {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
	}
}

func (m *Modes) printResult(sRows []Row) {

	if len(sRows) == 0 {
		return
	}

	keys := []string{"ipv4", "ipv6", "asn"}
	mSorted := SortRows(sRows)

	// walk groups
	for _, key := range keys {

		// get group rows
		spr := mSorted[key]
		if len(spr) == 0 {
			continue
		}

		// print rows
		if m.Pretty {
			for _, pRow := range spr {
				printPretty(os.Stdout, pRow)
			}
		} else {
			for _, pRow := range spr {
				printPlain(os.Stdout, pRow)
			}
		}
	}
}

func (m *Modes) doREPL(db *bbolt.DB, cmd string) error {

	row, assoc, err := m.execCmd(db, cmd)
	if err != nil {
		return err
	}

	// single-row OR all associated rows
	sRows := []Row{*row}
	if assoc {
		if sRows, err = row.FindAssociated(db); err != nil {
			return err
		}
	}

	m.printResult(sRows)
	return nil
}

func printPlainRow(out io.Writer, s [][]byte) error {
	if len(s) > 0 {
		_, err := out.Write(append(bytes.Join(s, []byte{'|'}), '\n'))
		return err
	}
	return nil
}

func printPlain(out io.Writer, pR *Row) error {

	if pR.IsType("asn") {
		return printPlainRow(out, [][]byte{
			pR.Registry, pR.Cc, pR.Type,
			pR.Start, pR.Value, pR.Date, pR.Status,
		})
	}

	for _, ipr := range pR.IpRange {
		err := printPlainRow(out, [][]byte{
			pR.Registry, pR.Cc, pR.Type,
			[]byte(ipr.String()), pR.Date, pR.Status,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type ColCfg struct {
	Txt []byte
	Wid int
	Rt  bool
}

func PrintCols(out io.Writer, part []ColCfg) error {

	var err error
	cpos := 1
	for ix, rc := range part {

		// right pad
		if rc.Rt {
			_, err = fmt.Fprintf(out, "\x1b[%dG", cpos+rc.Wid-len(rc.Txt))
			if err != nil {
				return err
			}
		}

		// data
		if _, err = out.Write(rc.Txt); err != nil {
			return err
		}

		// skip last column separator
		if ix == (len(part) - 1) {
			break
		}

		// track cursor position
		cpos += rc.Wid
		if _, err = fmt.Fprintf(out, "\x1b[%dG | ", cpos); err != nil {
			return err
		}
		cpos += 3
	}

	_, err = fmt.Fprintln(out, "")
	return err
}

func fmtDate(in []byte) []byte {
	if len(in) < 8 {
		return in
	}
	return bytes.Join([][]byte{in[:4], in[4:6], in[6:]}, []byte{'-'})
}

func printPretty(out io.Writer, pR *Row) error {

	if pR == nil {
		return nil
	}

	/*
		TODO:
			- decode country
			- test iprange looping (multiple CIDRs in one row)
	*/

	if pR.IsType("asn") {

		part := []ColCfg{
			ColCfg{Txt: pR.Registry, Wid: 9},
			ColCfg{Txt: pR.Cc, Wid: 3},
			ColCfg{Txt: pR.Type, Wid: 4},
			ColCfg{Txt: pR.Start, Wid: 10, Rt: true},
			ColCfg{Txt: pR.Value, Wid: 10, Rt: true},
			ColCfg{Txt: fmtDate(pR.Date), Wid: 10},
			ColCfg{Txt: pR.Status, Wid: 10},
		}

		if err := PrintCols(out, part); err != nil {
			return err
		}
	}

	// print row multiple times for each subnet
	for _, r := range pR.IpRange {

		if !r.IsValid() {
			continue
		}

		part := []ColCfg{
			ColCfg{Txt: pR.Registry, Wid: 9},
			ColCfg{Txt: pR.Cc, Wid: 3},
			ColCfg{Txt: pR.Type, Wid: 4},
			ColCfg{Txt: []byte(r.String()), Wid: 23, Rt: true},
			ColCfg{Txt: fmtDate(pR.Date), Wid: 10},
			ColCfg{Txt: pR.Status, Wid: 10},
		}

		if err := PrintCols(out, part); err != nil {
			return err
		}
	}

	return nil
}

var rxAssoc *regexp.Regexp = regexp.MustCompile(`^\s*(.*)\s*\|a\s*$`)
var rxASN *regexp.Regexp = regexp.MustCompile(`^\s*AS(\d+)\s*$`)

func (m *Modes) execCmd(db *bbolt.DB, cmd string) (
	row *Row, assoc bool, err error,
) {

	cmd = strings.TrimSpace(cmd)

	if m.UpdateFromCmd(cmd) {
		return
	}

	// fetch all associated
	const AllAssocSuffix = ":a"
	assoc = strings.HasSuffix(cmd, AllAssocSuffix)
	if assoc {
		cmd = strings.TrimSuffix(cmd, AllAssocSuffix)
	}

	// ASN
	sASN := rxASN.FindStringSubmatch(cmd)
	if len(sASN) > 1 {

		if nASN, e2 := strconv.ParseUint(sASN[1], 10, 32); e2 != nil {
			err = errors.WithMessage(e2, "invalid ASN")
		} else {
			row, err = FindByAsn(db, uint32(nASN))
		}
		return
	}

	// IP
	if ip, e2 := netip.ParseAddr(cmd); e2 == nil {
		row, err = FindByIp(db, ip)
		return
	}

	return nil, false, EInvalidQuery
}
