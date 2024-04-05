package main

/*
	TODO:
		- unit tests
		- embed parse regexs in command type
		- stress test regexes
*/

import (
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BourgeoisBear/nicsearch/rdap"
	"github.com/chzyer/readline"
	"github.com/mattn/go-isatty"
	"go.etcd.io/bbolt"
)

func (m *Modes) AnsiMsg(iWri io.Writer, title, msg string, sCsi []uint8) (int, error) {
	return m.AnsiMsgEx(iWri, title, msg, "", sCsi)
}

func (m *Modes) AnsiMsgEx(iWri io.Writer, title, msg, query string, sCsi []uint8) (int, error) {

	parts := make([]string, 0, 6)

	// prepend query to error in PrependQuery mode
	if m.PrependQuery && (len(query) > 0) {
		parts = append(parts, query, " | ")
	}

	// build/write title
	if m.Color && (len(sCsi) > 0) {
		sCodes := make([]string, len(sCsi))
		for ix := range sCsi {
			sCodes[ix] = strconv.Itoa(int(sCsi[ix]))
		}
		title = "\x1b[" + strings.Join(sCodes, ";") + "m" + title + "\x1b[0m"
	}
	parts = append(parts, title)

	if len(msg) > 0 {
		parts = append(parts, ": ", msg)
	}
	parts = append(parts, "\n")

	return iWri.Write([]byte(strings.Join(parts, "")))
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
			mode.printErr(E, "")
			os.Exit(1)
		}
	}()

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
	flag.BoolVar(&bReIndex, "reindex", false, "force rebuild of RIR database index")
	flag.BoolVar(&bDownload, "download", false, "force download of RIR databases")
	flag.BoolVar(&mode.Color, "color", bIsTty, "force color output on/off")
	flag.BoolVar(&mode.Pretty, "pretty", bIsTty, "force pretty print on/off")
	flag.BoolVar(&mode.PrependQuery, "prependQuery", false, "prepend query to corresponding result row in tabular outputs")
	flag.StringVar(&dbPath, "dbpath", dbPath, "override path to RIR data and index")

	var iWri io.Writer = os.Stdout
	flag.CommandLine.SetOutput(iWri)
	flag.Usage = func() {

		fmt.Fprint(iWri, `USAGE
  nicsearch [OPTION]... [QUERY]...

    Offline lookup by IP/ASN of other IPs/ASNs owned by the same organization.
    This tool can also dump IPs/ASNs by country code, as well as map most ASNs to
    their names.  Uses locally cached data, downloaded from all regional internet
    registries (RIRs) to prevent throttlings and timeouts on high-volume lookups.

OPTION
`)
		flag.PrintDefaults()

		fmt.Fprint(iWri, `
QUERY
  as ASN [+]
    query by autonomous system number (ASN).
      ex: 'as 14061'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
      ex: 'as 14061 +'

  ip IPADDR [+]
    query by IP (v4 or v6) address.
      ex: 'ip 172.104.6.84'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
      ex: 'ip 172.104.6.84 +'

  cc COUNTRY_CODE
    query by country code.
    returns all IPs & ASNs for the given country.
      ex: 'cc US'

  na REGEX [+]
    query by ASN name.
    returns all ASNs with names matching the given REGEX.
    see https://pkg.go.dev/regexp/syntax for syntax rules.
      ex: 'na microsoft'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization(s) of all matching ASNs.
      ex: 'na microsoft +'

  rdap.email IPADDR
    get email contacts for IPADDR.
      ex: 'rdap.email 8.8.8.8'

    NOTE: columns are separated by '@@' instead of '|' since pipe can
          appear inside the unquoted local-part of an email address.

  rdap.ip RIR IPADDR
    get full RDAP reply (in JSON) from RIR for IP address.
      ex: 'rdap.ip arin 8.8.8.8'

  rdap.org RIR ORGID
    get full RDAP reply (in JSON) from RIR for ORGID.
      ex: 'rdap.org arin DO-13'

  rdap.orgnets RIR ORGID
    an 'rdap.org' query, returning only the associated IP networks
    section in table format.
      ex: 'rdap.orgnets arin DO-13'

  all
    dump all local records

  NOTE: all 'rdap.' queries require an internet connection to the
        RIR's RDAP service.`)

		fmt.Fprint(iWri, "\n")
	}

	flag.Parse()

	// immediate exit on user-specified reindex/download without arg queries
	bExitOnCompletion := false
	if (bReIndex || bDownload) && (len(flag.Args()) == 0) {
		bExitOnCompletion = true
	}

	// create cache dir
	if E = os.MkdirAll(dbPath, 0775); E != nil {
		return
	}

	// build file list
	mDlItems := GetRIRDownloadItems(dbPath)
	sFiles := make([]DownloadItem, 0, len(mDlItems)+1)
	for _, di := range mDlItems {
		sFiles = append(sFiles, di)
	}

	/*
		TODO:
			- RIPE's list seems complete, but may want to merge
			  https://ftp.arin.net/info/asn.txt results, just in case.
					\d+\s+{RIR}-ASNBLOCK-\d+

			- find ASN name lists for other registries
	*/
	asnFile := DownloadItem{
		Host:    "ftp.ripe.net",
		SrcPath: "ripe/asnames/asn.txt",
		DstPath: filepath.Join(dbPath, "asn.txt.gz"),
	}
	sFiles = append(sFiles, asnFile)

	fnNotFound := func(fname string) (int, error) {
		return mode.AnsiMsg(os.Stderr, "NOT FOUND", fname, []uint8{1, 91})
	}
	// download delegations from each RIR & ASN list from RIPE
	for _, item := range sFiles {
		if bDownload || !Exists(item.DstPath) {
			if !bDownload {
				fnNotFound(item.DstPath)
			}
			if E = mode.DownloadAll(os.Stderr, item, dbPath); E != nil {
				return
			}
			bReIndex = true
		}
	}

	// force re-index if DB is not found
	boltDbFname := filepath.Join(dbPath, "nicsearch.db")
	if !bReIndex && !Exists(boltDbFname) {
		fnNotFound(boltDbFname)
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

		fnIndexing := func(fname string) (int, error) {
			return mode.AnsiMsg(os.Stderr, "INDEXING", fname, []uint8{1, 93})
		}

		// fill from sources
		for key := range mDlItems {
			fname := mDlItems[key].DstPath
			fnIndexing(fname)
			if E = pBkt.GzRead(fname, pBkt.scanlnDelegation); E != nil {
				return
			}
		}

		// fill ASN lookup
		fname := asnFile.DstPath
		fnIndexing(fname)
		E = pBkt.GzRead(fname, pBkt.scanlnAsName)

		if bExitOnCompletion {
			return
		}
	}

	// command REPL
	sCmds := flag.Args()
	if len(sCmds) == 0 {

		// stdin command mode
		rl, e2 := readline.New("> ")
		if e2 != nil {
			E = e2
			return
		}
		defer rl.Close()

		for {
			line, e2 := rl.Readline()
			if e2 != nil {
				E = e2
				return
			}

			runeLen := utf8.RuneCountInString(line)
			if e2 := mode.doREPL(db, line, runeLen); e2 != nil {
				mode.printErr(e2, line)
			}
		}

	} else {

		// get char length of longest query
		runeLen := 0
		for ix := range sCmds {
			n := utf8.RuneCountInString(sCmds[ix])
			if n > runeLen {
				runeLen = n
			}
		}

		// args command mode
		for ix := range sCmds {
			if err := mode.doREPL(db, sCmds[ix], runeLen); err != nil {
				mode.printErr(err, sCmds[ix])
			}
		}
	}

	return
}

func (m *Modes) printErr(err error, query string) (int, error) {
	if err == nil {
		return 0, nil
	}
	return m.AnsiMsgEx(os.Stderr, "error", err.Error(), query, []uint8{1, 91})
}

func (m *Modes) doREPL(db *bbolt.DB, szCmd string, maxCmdLen int) error {

	szCmd = strings.ToUpper(strings.TrimSpace(szCmd))
	iCmd, err := m.ParseCmd(szCmd)
	if err != nil {
		return err
	}

	return iCmd.Exec(
		CmdExecParams{
			Modes:     *m,
			Db:        db,
			Cmd:       szCmd,
			MaxCmdLen: uint16(maxCmdLen),
		},
	)
}

func rdapByIp(db *bbolt.DB, ip netip.Addr) ([]byte, error) {

	row, err := IpToRow(db, ip)
	if err != nil {
		return nil, err
	}

	szReg := string(row.Registry)
	rk, err := rdap.RegistryNameToKey(szReg)
	if err != nil {
		return nil, err
	}

	return rdap.QueryByIP(rk, ip)
}
