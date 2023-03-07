package main

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type CmdIP struct {
	IP    netip.Addr
	Assoc bool
}

type CmdASN struct {
	ASN   uint32
	Assoc bool
}

type CmdCC struct {
	CC string
}

var g_rxASN, g_rxCC *regexp.Regexp

func init() {
	g_rxASN = regexp.MustCompile(`^\s*AS(\d+)\s*$`)
	g_rxCC = regexp.MustCompile(`^\s*CC([A-Z]{2})\s*$`)
}

func (m *Modes) ParseCmd(cmd string) (interface{}, error) {

	// fetch all associated
	const AllAssocSuffix = ",a"
	assoc := strings.HasSuffix(cmd, AllAssocSuffix)
	if assoc {
		cmd = strings.TrimSuffix(cmd, AllAssocSuffix)
	}

	// ASN
	sASN := g_rxASN.FindStringSubmatch(cmd)
	if len(sASN) > 1 {
		if nASN, e2 := strconv.ParseUint(sASN[1], 10, 32); e2 != nil {
			return nil, errors.WithMessage(e2, "invalid ASN")
		} else {
			return CmdASN{ASN: uint32(nASN), Assoc: assoc}, nil
		}
	}

	// COUNTRY CODE
	sCC := g_rxCC.FindStringSubmatch(cmd)
	if len(sCC) > 1 {
		return CmdCC{CC: strings.ToUpper(sCC[1])}, nil
	}

	// IP
	if ip, e2 := netip.ParseAddr(cmd); e2 == nil {
		return CmdIP{IP: ip, Assoc: assoc}, nil
	}

	return nil, EInvalidQuery
}
