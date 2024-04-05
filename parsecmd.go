package main

import (
	"encoding/json"
	"io"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/BourgeoisBear/nicsearch/rdap"
	"github.com/pkg/errors"
)

type CmdIP struct {
	IP    netip.Addr
	Assoc bool
}

type CmdEmail struct {
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

type CmdRDAP_IP struct {
	RIR rdap.RIRKey
	IP  netip.Addr
}

type CmdRDAP_Org struct {
	RIR      rdap.RIRKey
	OrgId    string
	NetsOnly bool
}

type CmdAll struct{}

var g_cmdRegex []*regexp.Regexp

func init() {

	szRegex := []string{
		`(AS)N?\s+(\d+)\s*(\s\+)?`,
		`(IP)\s+(.*?)\s*(\s\+)?`,
		`(NA)(?:ME)?\s+(.*?)\s*(\s\+)?`,
		`(CC)\s+([A-Z]{2})\s*`,
		`(ALL)\s*`,
		`(EMAIL)\s+(.*?)\s*`,
		`(RDAP\.IP)\s+(.*?)\s+(.*?)\s*`,
		`(RDAP\.ORG)\s+(.*?)\s+(.*?)\s*`,
		`(RDAP\.ORGNETS)\s+(.*?)\s+(.*?)\s*`,
	}

	for _, txt := range szRegex {
		g_cmdRegex = append(
			g_cmdRegex,
			regexp.MustCompile(`(?i)^\s*`+txt+`$`),
		)
	}
}

type Modes struct {
	Color        bool
	Pretty       bool
	PrependQuery bool
}

func (m *Modes) PrintJSON(iWri io.Writer, bsJSON []byte) error {

	// unmarshal / re-marshal + indent in pretty mode
	if m.Pretty {

		mTmp := make(map[string]interface{})
		err := json.Unmarshal(bsJSON, &mTmp)
		if err != nil {
			return err
		}

		bsJSON, err = json.MarshalIndent(mTmp, "", "\t")
		if err != nil {
			return err
		}
	}

	_, err := iWri.Write(append(bsJSON, '\n'))
	return err
}

// cmd should be upper-case and without leading/trailing whitespace
func (m *Modes) ParseCmd(cmd string) (CmdExec, error) {

	// TODO: quick command reference, point out closest command on fail

	for _, rx := range g_cmdRegex {

		sArg := rx.FindStringSubmatch(cmd)
		if len(sArg) < 2 {
			continue
		}
		sArg = sArg[1:]

		// TODO: validate commands
		// TODO: stress test regexes
		bGetAssociated := strings.HasSuffix(cmd, "+")

		switch sArg[0] {

		// ASN
		case "AS":
			nASN, e2 := strconv.ParseUint(sArg[1], 10, 32)
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid ASN")
			}
			return CmdASN{ASN: uint32(nASN), Assoc: bGetAssociated}, nil

		// IP
		case "IP":
			ip, e2 := netip.ParseAddr(sArg[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdIP{IP: ip, Assoc: bGetAssociated}, nil

		// NA
		case "NA":
			if len(sArg[1]) == 0 {
				return nil, errors.New("empty name search regex")
			}
			return CmdAsName{Name: sArg[1], Assoc: bGetAssociated}, nil

		// CC
		case "CC":
			return CmdCC{CC: sArg[1]}, nil

		// ALL
		case "ALL":
			return CmdAll{}, nil

		// EMAIL
		case "EMAIL":
			ip, e2 := netip.ParseAddr(sArg[1])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}
			return CmdEmail{IP: ip}, nil

		// RDAP
		case "RDAP.IP":

			rk, e2 := rdap.RegistryNameToKey(sArg[1])
			if e2 != nil {
				return nil, e2
			}

			ip, e2 := netip.ParseAddr(sArg[2])
			if e2 != nil {
				return nil, errors.WithMessage(e2, "invalid IP")
			}

			return CmdRDAP_IP{RIR: rk, IP: ip}, nil

		case "RDAP.ORG":

			rk, e2 := rdap.RegistryNameToKey(sArg[1])
			if e2 != nil {
				return nil, e2
			}
			return CmdRDAP_Org{RIR: rk, OrgId: sArg[2]}, nil

		case "RDAP.ORGNETS":

			rk, e2 := rdap.RegistryNameToKey(sArg[1])
			if e2 != nil {
				return nil, e2
			}
			return CmdRDAP_Org{RIR: rk, OrgId: sArg[2], NetsOnly: true}, nil
		}
	}

	return nil, EInvalidQuery
}
