package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/BourgeoisBear/range2cidr"
)

func main() {

	/*
		TODO:
			- iterate dbs
			- lookup by ip
			- lookup by asn
	*/

	if false {
		range2cidr.Deaggregate(netip.Addr{}, netip.Addr{})
	}

	var E error
	defer func() {
		if E != nil {
			fmt.Println(E)
			os.Exit(1)
		}
	}()

	// TODO: flags (by ip, by asn, specify registries)
	// TODO: download DBs & compress
	// TODO: IP mode: iterate each DB, stop on first match
	// TODO: ASN mode: iterate specified DB
	pF, E := os.Open("./db/arin_latest.txt.gz")
	if E != nil {
		return
	}
	defer pF.Close()

	// gunzip
	gzr, E := gzip.NewReader(pF)
	if E != nil {
		return
	}

	// store rows in memory
	lines := make([]Row, 0)
	pSc := bufio.NewScanner(gzr)
	ixLine := 0
	for pSc.Scan() {

		ixLine += 1
		szLine := pSc.Text()

		// skip header
		if ixLine == 1 {
			continue
		}

		// skip comments
		if strings.HasPrefix(szLine, "#") {
			continue
		}

		// skip summaries
		if strings.HasSuffix(szLine, "|summary") {
			continue
		}

		pr, err := ParseRow(szLine)
		if err != nil {
			err = fmt.Errorf("Line %d: %s\nError: %w", ixLine, szLine, err)
			fmt.Fprintln(os.Stderr, err)
		} else {
			lines = append(lines, pr)
		}
	}
	if E = pSc.Err(); E != nil {
		return
	}

	regId, E := RegIdFromIP(lines, "23.239.224.0")
	if E != nil {
		return
	}
	fmt.Println(regId)

	oAsn, found := LookupASN(lines, "7903")
	if found {
		PrintASN(os.Stdout, oAsn)
	}

	if len(regId) > 0 {
		// TODO: as callback, capture ASN(s)
		LookupRegId(lines, regId)
	}
}

func RegIdFromIP(lines []Row, szTgtIp string) (string, error) {

	ipTgtIp, err := netip.ParseAddr(szTgtIp)
	if err != nil {
		return "", err
	}

	var tgt string
	if ipTgtIp.Is4In6() {
		ipTgtIp = ipTgtIp.Unmap()
		tgt = "ipv4"
	} else if ipTgtIp.Is4() {
		tgt = "ipv4"
	} else if ipTgtIp.Is6() {
		tgt = "ipv6"
	} else {
		return "", errors.New("UNKNOWN ADDR TYPE")
	}

	for _, row := range lines {

		// version mismatch
		if row.Type != tgt {
			continue
		}

		// no network prefixes
		if len(row.IpRange) == 0 {
			continue
		}

		for _, rng := range row.IpRange {
			if rng.IsValid() && rng.Contains(ipTgtIp) {
				return row.RegId, nil
			}
		}
	}

	return "", nil
}

func LookupRegId(ln []Row, regId string) {
	for ix := range ln {
		if ln[ix].RegId != regId {
			continue
		}
		fmt.Println(ln[ix].Pretty())
	}
}

func LookupASN(ln []Row, asnId string) (Row, bool) {
	for ix := range ln {
		if (ln[ix].Type != "asn") || (ln[ix].Start != asnId) {
			continue
		}
		return ln[ix], true
	}
	return Row{}, false
}

type Row struct {
	Registry string
	Cc       string
	Type     string
	Start    string
	Value    string
	Date     string
	Status   string
	RegId    string

	ValueInt int
	IpStart  netip.Addr
	IpRange  []netip.Prefix
}

func (pR *Row) Raw() string {
	s := []string{
		pR.Registry,
		pR.Cc,
		pR.Type,
		pR.Start,
		pR.Value,
		pR.Date,
		pR.Status,
		pR.RegId,
	}
	return strings.Join(s, "|")
}

func (pR *Row) Pretty() string {

	s := make([]string, 0, 7)
	s = append(s, pR.Registry, pR.Cc, pR.Type)

	if pR.Type == "asn" {

		s = append(s, pR.Start, pR.Value)

	} else {

		if len(pR.IpRange) > 0 {
			sRng := make([]string, 0, len(pR.IpRange))
			for _, r := range pR.IpRange {
				if r.IsValid() {
					sRng = append(sRng, r.String())
				}
			}
			if len(sRng) > 0 {
				s = append(s, strings.Join(sRng, ","))
			}
		}
	}

	s = append(s, pR.Date, pR.Status)

	return strings.Join(s, "|")
}

func ParseRow(line string) (ret Row, err error) {

	row := strings.Split(line, "|")

	for ix := range row {
		val := strings.TrimSpace(row[ix])
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

	nVal, err := strconv.Atoi(ret.Value)
	if err != nil {
		err = fmt.Errorf("col 5, number expected: %w", err)
		return
	}
	ret.ValueInt = nVal

	switch ret.Type {
	case "ipv4", "ipv6":
		ip, e2 := netip.ParseAddr(ret.Start)
		if e2 != nil {
			err = fmt.Errorf("col 4, ip addr expected: %w", e2)
			return
		}
		ret.IpStart = ip
		// TODO: validate that address is of correct family

		switch ret.Type {
		case "ipv6":
			ret.IpRange = []netip.Prefix{netip.PrefixFrom(ip, nVal)}

		case "ipv4":

			// TODO: tabularize in TTY mode
			// ls *.go *.gz | entr -c go run nicsearch.go

			// get base addr as int
			uBase, ok := range2cidr.V4ToUint32(ip)
			if !ok {
				err = errors.New("failed to convert v4 address to uint32")
				return
			}

			// calculate last host addr, then convert back to netip
			ipLast := range2cidr.Uint32ToV4(uBase + uint32(nVal) - 1)

			// create network masks from range
			ret.IpRange, e2 = range2cidr.Deaggregate(ip, ipLast)
			if e2 != nil {
				err = fmt.Errorf("ip deaggregation failure: %w", e2)
				return
			}
		}
	}

	return
}

func PrintASN(iWri io.Writer, data Row) {
	// TODO: improved formatting
	fmt.Fprintf(iWri, "%#v\n", data)
}
