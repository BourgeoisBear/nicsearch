package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
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

var g_colWri ColWriter
var g_ccfgASN, g_ccfgIP []ColCfg

func init() {

	g_colWri = ColWriter{}

	g_ccfgASN = []ColCfg{
		ColCfg{Wid: 9},
		ColCfg{Wid: 3},
		ColCfg{Wid: 4},
		ColCfg{Wid: 10, Rt: true},
		ColCfg{Wid: 10, Rt: true},
		ColCfg{Wid: 10},
		ColCfg{Wid: 10},
		ColCfg{},
	}

	g_ccfgIP = []ColCfg{
		ColCfg{Wid: 9},
		ColCfg{Wid: 3},
		ColCfg{Wid: 4},
		ColCfg{Wid: 23, Rt: true},
		ColCfg{Wid: 10},
		ColCfg{Wid: 10},
	}
}

func Exists(fname string) bool {
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		return false
	}
	return true
}

// ls *.go *.gz | entr -c go run nicsearch.go
// Ctrl-w N (copy)
// Ctrl-w "+ (paste)
func main() {

	var E error
	var mode Modes
	defer func() {
		if E != nil {
			mode.printErr(E)
			os.Exit(1)
		}
	}()

	// TODO: test on windows

	// default to pretty & color if TTY
	bIsTty := false
	if isatty.IsTerminal(os.Stdout.Fd()) {
		bIsTty = true
	}

	// default paths
	dbPath, E := os.UserHomeDir()
	if E != nil {
		return
	}
	szAppName := filepath.Base(os.Args[0])
	ext := filepath.Ext(szAppName)
	if len(ext) > 0 {
		szAppName = strings.TrimSuffix(szAppName, ext)
	}
	dbPath = filepath.Join(dbPath, ".cache", szAppName)

	// flags
	var bReIndex, bDownload bool
	flag.BoolVar(&bReIndex, "reindex", false, "rebuild RIR database index")
	flag.BoolVar(&bDownload, "download", false, "download RIR databases")
	flag.BoolVar(&mode.Color, "color", bIsTty, "force color output on/off")
	flag.BoolVar(&mode.Pretty, "pretty", bIsTty, "force pretty print on/off")
	flag.StringVar(&dbPath, "dbpath", dbPath, "path to RIR data and index")
	flag.Parse()

	// create cache dir
	if E = os.MkdirAll(dbPath, 0775); E != nil {
		return
	}

	// build file list
	asnFile := DownloadItem{
		Host:    "ftp.ripe.net",
		SrcPath: "ripe/asnames/asn.txt",
		DstPath: filepath.Join(dbPath, "asn.txt.gz"),
	}
	mDlItems := GetRIRDownloadItems(dbPath)
	sFiles := make([]DownloadItem, 0, len(mDlItems)+1)
	for _, di := range mDlItems {
		sFiles = append(sFiles, di)
	}
	sFiles = append(sFiles, asnFile)

	// download delegations from each RIR & ASN list from RIPE
	for _, item := range sFiles {
		if bDownload || !Exists(item.DstPath) {
			if !bDownload {
				fmt.Fprintf(os.Stderr, "'%s' NOT FOUND: DOWNLOADING RIR DATA\n", item.DstPath)
			}
			if E = DownloadAll(os.Stderr, item, dbPath); E != nil {
				return
			}
			bReIndex = true
		}
	}

	// force re-index if DB is not found
	boltDbFname := filepath.Join(dbPath, "nicsearch.db")
	if !bReIndex && !Exists(boltDbFname) {
		fmt.Fprintf(os.Stderr, "'%s' NOT FOUND: RE-INDEXING\n", boltDbFname)
		bReIndex = true
	}

	// init boltdb
	if bReIndex {
		os.Remove(boltDbFname)
	}
	db, E := bbolt.Open(boltDbFname, 0664, nil)
	if E != nil {
		return
	}
	defer db.Close()

	// rebuild index
	if bReIndex {

		// create buckets
		pBkt, err := CreateBktFiller(db)
		if err != nil {
			E = err
			return
		}

		// fill from sources
		for key := range mDlItems {
			fname := mDlItems[key].DstPath
			fmt.Fprintf(os.Stderr, "\x1b[1mINDEXING:\x1b[0m %s\n", fname)
			if E = pBkt.GzRead(fname, pBkt.scanlnDelegation); E != nil {
				return
			}
		}

		// fill ASN lookup
		fname := asnFile.DstPath
		fmt.Fprintf(os.Stderr, "\x1b[1mINDEXING:\x1b[0m %s\n", fname)
		E = pBkt.GzRead(fname, pBkt.scanlnAsName)
	}

	// command REPL
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

func (m *Modes) printRowsSorted(db *bbolt.DB, sRows []Row) error {

	if len(sRows) == 0 {
		return nil
	}

	keys := []string{"ipv4", "ipv6", "asn"}
	mSorted := SortRows(sRows)

	// lookup asnames
	sASN := mSorted["asn"]
	var err error
	for ix := range sASN {
		sASN[ix].AsName, err = FindAsName(db, sASN[ix].ASN)
		if err != nil {
			return err
		}
	}

	// walk groups
	for _, key := range keys {

		// get group rows
		spr := mSorted[key]
		if len(spr) == 0 {
			continue
		}

		// print rows
		for _, pRow := range spr {
			if err := m.printRow(os.Stdout, pRow); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Modes) doREPL(db *bbolt.DB, cmd string) error {

	cmd = strings.TrimSpace(cmd)
	if m.UpdateFromCmd(cmd) {
		return nil
	}

	iCmd, err := m.ParseCmd(cmd)
	if err != nil {
		return err
	}

	var row *Row
	var assoc bool
	switch v := iCmd.(type) {
	case CmdIP:
		assoc = v.Assoc
		row, err = FindByIp(db, v.IP)
	case CmdASN:
		assoc = v.Assoc
		row, err = FindByAsn(db, v.ASN)
	case CmdCC:
		bsCC := []byte("|" + v.CC + "|")
		return WalkRawRows(db, func(k, v []byte) error {
			if !bytes.Contains(v, bsCC) {
				return nil
			}
			if row, e2 := ParseRow(v, true); e2 != nil {
				return e2
			} else {
				return m.printRow(os.Stdout, &row)
			}
		})
	}

	if row == nil {
		return nil
	}
	sRows := []Row{*row}
	if assoc {
		if sRows, err = row.FindAssociated(db); err != nil {
			return err
		}
	}
	return m.printRowsSorted(db, sRows)
}

func fmtDate(in []byte) []byte {
	if len(in) < 8 {
		return in
	}
	return bytes.Join([][]byte{in[:4], in[4:6], in[6:]}, []byte{'-'})
}

func (m *Modes) printRow(out io.Writer, pR *Row) error {

	if pR == nil {
		return nil
	}

	var wriCfg ColWriterCfg
	if m.Pretty {
		wriCfg = ColWriterCfg{Pad: true, Spacer: []byte(" | ")}
	} else {
		wriCfg = ColWriterCfg{Pad: false, Spacer: []byte("|")}
	}

	if pR.IsType("asn") {

		return g_colWri.WriteCols(
			out, wriCfg, g_ccfgASN,
			pR.Registry,
			pR.Cc,
			pR.Type,
			pR.Start,
			pR.Value,
			fmtDate(pR.Date),
			pR.Status,
			pR.AsName,
		)
	}

	// print row multiple times for each subnet
	for _, r := range pR.IpRange {

		if !r.IsValid() {
			continue
		}

		err := g_colWri.WriteCols(
			out, wriCfg, g_ccfgIP,
			pR.Registry,
			pR.Cc,
			pR.Type,
			[]byte(r.String()),
			fmtDate(pR.Date),
			pR.Status,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
