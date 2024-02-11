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

type CmdAbuse struct {
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

type Modes struct {
	Color        bool
	Pretty       bool
	PrependQuery bool
	CmdRegex     []*regexp.Regexp
}

func (m *Modes) ParseCmd(cmd string) (interface{}, error) {

	// build regexes on first invocation
	if len(m.CmdRegex) == 0 {

		sSyntax := []string{
			`^\s*AS\s+(\d+)\s*(\s\+)?$`,
			`^\s*IP\s+(.*?)\s*(\s\+)?$`,
			`^\s*NA\s+(.*?)\s*(\s\+)?$`,
			`^\s*CC\s+([A-Z]{2})\s*$`,
			`^\s*ALL\s*$`,
			`^\s*EMAIL\s+(.*?)\s*$`,
		}
		var err error
		m.CmdRegex = make([]*regexp.Regexp, len(sSyntax))
		for ix, txt := range sSyntax {
			m.CmdRegex[ix], err = regexp.Compile(`(?i)` + txt)
			if err != nil {
				return nil, err
			}
		}
	}

	for ix, rx := range m.CmdRegex {

		sMtch := rx.FindStringSubmatch(cmd)
		if len(sMtch) == 0 {
			continue
		}

		// fmt.Printf("%#v\n", sMtch)
		// return nil, EInvalidQuery

		var assoc bool
		if len(sMtch) == 3 {
			assoc = len(sMtch[2]) > 0
		}

		switch ix {

		// ASN
		case 0:
			nASN, e2 := strconv.ParseUint(sMtch[1], 10, 32)
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid ASN")
			}
			return CmdASN{ASN: uint32(nASN), Assoc: assoc}, nil

		// IP
		case 1:
			ip, e2 := netip.ParseAddr(sMtch[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdIP{IP: ip, Assoc: assoc}, nil

		// NA
		case 2:
			if len(sMtch[1]) == 0 {
				return nil, errors.New("empty name search regex")
			}
			return CmdAsName{Name: sMtch[1], Assoc: assoc}, nil

		// CC
		case 3:
			return CmdCC{CC: sMtch[1]}, nil

		// ALL
		case 4:
			return CmdAll{}, nil

		// EMAILS
		case 5:
			ip, e2 := netip.ParseAddr(sMtch[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdAbuse{IP: ip}, nil
		}
	}

	return nil, EInvalidQuery
}
