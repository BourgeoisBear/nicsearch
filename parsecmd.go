package main

import (
	"net/netip"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

type CmdIP struct {
	IP    netip.Addr
	Assoc bool
}

type CmdEmail struct {
	IP netip.Addr
}

type CmdRDAP struct {
	IP netip.Addr
}

type CmdASN struct {
	ASN   uint32
	Assoc bool
}

type CmdCC struct {
	CC string
}

type CmdAsName struct {
	Name  string
	Assoc bool
}

type CmdAll struct{}

type CmdIx int

const (
	CmdIxASN CmdIx = iota
	CmdIxIP
	CmdIxNAME
	CmdIxCC
	CmdIxALL
	CmdIxEMAIL
	CmdIxRDAP
	CmdIxMAX
)

type Modes struct {
	Color        bool
	Pretty       bool
	PrependQuery bool
	CmdRegex     map[CmdIx]*regexp.Regexp
}

func (m *Modes) ParseCmd(cmd string) (interface{}, error) {

	// build regexes on first invocation
	if len(m.CmdRegex) == 0 {

		sSyntax := map[CmdIx]string{
			CmdIxASN:   `^\s*ASN?\s+(\d+)\s*(\s\+)?$`,
			CmdIxIP:    `^\s*IP\s+(.*?)\s*(\s\+)?$`,
			CmdIxNAME:  `^\s*NA(?:ME)?\s+(.*?)\s*(\s\+)?$`,
			CmdIxCC:    `^\s*CC\s+([A-Z]{2})\s*$`,
			CmdIxALL:   `^\s*ALL\s*$`,
			CmdIxEMAIL: `^\s*EMAIL\s+(.*?)\s*$`,
			CmdIxRDAP:  `^\s*RDAP\s+(.*?)\s*$`,
		}
		var err error
		m.CmdRegex = make(map[CmdIx]*regexp.Regexp, CmdIxMAX)
		for key, txt := range sSyntax {
			m.CmdRegex[key], err = regexp.Compile(`(?i)` + txt)
			if err != nil {
				return nil, err
			}
		}
	}

	for rxKey, rx := range m.CmdRegex {

		sMtch := rx.FindStringSubmatch(cmd)
		if len(sMtch) == 0 {
			continue
		}

		var assoc bool
		if len(sMtch) == 3 {
			assoc = len(sMtch[2]) > 0
		}

		switch rxKey {

		// ASN
		case CmdIxASN:
			nASN, e2 := strconv.ParseUint(sMtch[1], 10, 32)
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid ASN")
			}
			return CmdASN{ASN: uint32(nASN), Assoc: assoc}, nil

		// IP
		case CmdIxIP:
			ip, e2 := netip.ParseAddr(sMtch[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdIP{IP: ip, Assoc: assoc}, nil

		// NA
		case CmdIxNAME:
			if len(sMtch[1]) == 0 {
				return nil, errors.New("empty name search regex")
			}
			return CmdAsName{Name: sMtch[1], Assoc: assoc}, nil

		// CC
		case CmdIxCC:
			return CmdCC{CC: sMtch[1]}, nil

		// ALL
		case CmdIxALL:
			return CmdAll{}, nil

		// EMAIL
		case CmdIxEMAIL:
			ip, e2 := netip.ParseAddr(sMtch[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdEmail{IP: ip}, nil

		// RDAP
		case CmdIxRDAP:
			ip, e2 := netip.ParseAddr(sMtch[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdRDAP{IP: ip}, nil
		}
	}

	return nil, EInvalidQuery
}
