package main

import (
	"bytes"
	"encoding/json"
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

var g_colWri ColWriter
var g_ccfgASN, g_ccfgIP, g_ccfgEmail []ColCfg

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

	g_ccfgEmail = []ColCfg{
		ColCfg{Wid: 16},
		ColCfg{Wid: 16},
		ColCfg{},
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
	flag.BoolVar(&mode.PrependQuery, "prependQuery", false, "prepend query to corresponding result row")
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
    example: 'as 14061'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
    example: 'as 14061 +'

  ip IPADDR [+]
    query by IP (v4 or v6) address.
    example: 'ip 172.104.6.84'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
    example: 'ip 172.104.6.84 +'

  cc COUNTRY_CODE
    query by country code.  returns all IPs & ASNs for the given country.
    example: 'cc US'

  na REGEX [+]
    query by ASN name.  returns all ASNs with names matching the given REGEX.
    see https://pkg.go.dev/regexp/syntax for syntax rules.
    example: 'na microsoft'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization(s) of all matching ASNs.
    example: 'na microsoft +'

  email IPADDR
    get email contacts for IPADDR
    example: 'email 8.8.8.8'

    NOTE: this is an on-line query against the RIR's RDAP service.
          columns are separated by '@@' instead of '|' since
          pipe can appear inside the unquoted local-part of an email address.

  rdap IPADDR
    get full RDAP reply for IPADDR
    example: 'rdap 8.8.8.8'

    NOTE: this is an on-line query against the RIR's RDAP service.

  all
    dump all records`)

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

	// TODO: add https://ftp.arin.net/info/asn.txt
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

func (m *Modes) printRowsSorted(db *bbolt.DB, sRows []Row, cmd string, maxCmdLen int) error {

	if len(sRows) == 0 {
		return nil
	}

	keys := []TypeKey{TkIP4, TkIP6, TkASN}
	mSorted := SortRows(sRows)

	// lookup asnames
	sASN := mSorted[TkASN]
	var err error
	for ix := range sASN {

		if len(sASN[ix].AsName) > 0 {
			continue
		}

		sASN[ix].AsName, err = AsnToName(db, sASN[ix].ASN)
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
			if err := m.printRow(os.Stdout, pRow, cmd, maxCmdLen); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Modes) doREPL(db *bbolt.DB, cmd string, maxCmdLen int) error {

	cmd = strings.TrimSpace(cmd)
	iCmd, err := m.ParseCmd(cmd)
	if err != nil {
		return err
	}

	fnPrintResult := func(row *Row, bAssoc bool) error {
		if !bAssoc {
			return m.printRowsSorted(db, []Row{*row}, cmd, maxCmdLen)
		}
		sRows, err := FindAssociated(db, row.Registry, row.RegId)
		if err != nil {
			return err
		}
		return m.printRowsSorted(db, sRows, cmd, maxCmdLen)
	}

	switch v := iCmd.(type) {
	case CmdAsName:
		sRows, err := NameRegexToASNs(db, v.Name)
		if err != nil {
			return err
		}
		if len(sRows) == 0 {
			return ENotFound
		}
		if v.Assoc {
			// get unique reg-id keypairs
			byRegId, sKeys := UniqueRegIds(sRows)

			// collect associateds
			sRows = nil
			for _, k := range sKeys {
				pr := byRegId[k]
				sTmp, err := FindAssociated(db, pr.Registry, pr.RegId)
				if err != nil {
					return err
				}
				sRows = append(sRows, sTmp...)
			}
		}

		// print collection
		return m.printRowsSorted(db, sRows, cmd, maxCmdLen)

	case CmdIP:
		if row, err := IpToRow(db, v.IP); err != nil {
			return err
		} else {
			return fnPrintResult(&row, v.Assoc)
		}

	case CmdEmail:

		bsJSON, err := rdapByIp(db, v.IP)
		if err != nil {
			return err
		}

		var ent rdap.Entity
		err = json.Unmarshal(bsJSON, &ent)
		if err != nil {
			os.Stderr.Write(bsJSON)
			return err
		}

		for _, ea := range ent.GetEmailAddrs() {
			m.printEmailAddr(os.Stdout, ea, cmd, maxCmdLen)
		}

	case CmdRDAP:

		bsJSON, err := rdapByIp(db, v.IP)
		if err != nil {
			return err
		}

		// unmarshal / re-marshal + indent in pretty mode
		if m.Pretty {

			mTmp := make(map[string]interface{})
			err = json.Unmarshal(bsJSON, &mTmp)
			if err != nil {
				return err
			}

			bsJSON, err = json.MarshalIndent(mTmp, "", "\t")
			if err != nil {
				return err
			}
		}

		_, err = os.Stdout.Write(bsJSON)
		return err

	case CmdASN:
		if row, err := AsnToRow(db, v.ASN); err != nil {
			return err
		} else {
			return fnPrintResult(&row, v.Assoc)
		}

	case CmdCC:
		bsCC := []byte("|" + strings.ToUpper(v.CC) + "|")
		nFound := 0
		err := WalkRawRows(db, func(_, bsData []byte) error {
			if !bytes.Contains(bsData, bsCC) {
				return nil
			}
			if row, e2 := ParseRow(bsData); e2 != nil {
				return e2
			} else {
				nFound += 1
				return m.printRow(os.Stdout, &row, cmd, maxCmdLen)
			}
		})
		if err != nil {
			return err
		}
		if nFound == 0 {
			return ENotFound
		}

	case CmdAll:
		return WalkRawRows(db, func(_, bsData []byte) error {
			if row, e2 := ParseRow(bsData); e2 != nil {
				return e2
			} else {
				return m.printRow(os.Stdout, &row, cmd, maxCmdLen)
			}
		})
	}

	return nil
}

func (m *Modes) printRow(out io.Writer, pR *Row, cmd string, maxCmdLen int) error {

	if pR == nil {
		return nil
	}

	fmtDate := func(in []byte) []byte {
		if !m.Pretty || (len(in) < 8) {
			return in
		}
		return bytes.Join([][]byte{in[:4], in[4:6], in[6:]}, []byte{'-'})
	}

	var wriCfg ColWriterCfg
	if m.Pretty {
		wriCfg = ColWriterCfg{Pad: true, Spacer: []byte(" | ")}
	} else {
		wriCfg = ColWriterCfg{Pad: false, Spacer: []byte("|")}
	}

	fnPrependQuery := func(cfgs []ColCfg, txts ...[]byte) ([]ColCfg, [][]byte) {
		if m.PrependQuery {
			return append([]ColCfg{{Wid: maxCmdLen, Rt: false}}, cfgs...),
				append([][]byte{[]byte(cmd)}, txts...)
		} else {
			return cfgs, txts
		}
	}

	if pR.IsType(TkASN) {

		szAsnFirst := strconv.FormatInt(int64(pR.ASN), 10)
		szAsnLast := ""
		if pR.ValueInt > 1 {
			szAsnLast = strconv.FormatInt(
				int64(pR.ASN)+int64(pR.ValueInt)-1,
				10,
			)
		}

		ccfg, sFields := fnPrependQuery(
			g_ccfgASN,
			pR.Registry,
			pR.Cc,
			pR.Type,
			[]byte(szAsnFirst),
			[]byte(szAsnLast),
			fmtDate(pR.Date),
			pR.Status,
			pR.AsName,
		)

		return g_colWri.WriteCols(out, wriCfg, ccfg, sFields...)
	}

	// print row multiple times for each subnet
	for _, r := range pR.IpRange {

		if !r.IsValid() {
			continue
		}

		ccfg, sFields := fnPrependQuery(
			g_ccfgIP,
			pR.Registry,
			pR.Cc,
			pR.Type,
			[]byte(r.String()),
			fmtDate(pR.Date),
			pR.Status,
		)

		err := g_colWri.WriteCols(out, wriCfg, ccfg, sFields...)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Modes) printEmailAddr(out io.Writer, em rdap.EntityEmail, cmd string, maxCmdLen int) error {

	var wriCfg ColWriterCfg
	if m.Pretty {
		wriCfg = ColWriterCfg{Pad: true, Spacer: []byte(" @@ ")}
	} else {
		wriCfg = ColWriterCfg{Pad: false, Spacer: []byte("@@")}
	}

	fnPrependQuery := func(cfgs []ColCfg, txts ...[]byte) ([]ColCfg, [][]byte) {
		if m.PrependQuery {
			return append([]ColCfg{{Wid: maxCmdLen, Rt: false}}, cfgs...),
				append([][]byte{[]byte(cmd)}, txts...)
		} else {
			return cfgs, txts
		}
	}

	ccfg, sFields := fnPrependQuery(
		g_ccfgEmail,
		[]byte(em.Role),
		[]byte(em.Handle),
		[]byte(em.Addr),
	)

	return g_colWri.WriteCols(out, wriCfg, ccfg, sFields...)
}

func rdapByIp(db *bbolt.DB, ip netip.Addr) ([]byte, error) {

	row, err := IpToRow(db, ip)
	if err != nil {
		return nil, err
	}

	szReg := string(row.Registry)
	rk := rdap.RegistryNameToKey(szReg)
	if rk == rdap.RkMAX {
		return nil, fmt.Errorf("'%s' is not a valid registry name", szReg)
	}

	return rdap.QueryRDAPByIP(rk, ip)
}
