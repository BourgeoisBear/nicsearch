package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/BourgeoisBear/nicsearch/rdap"
	"github.com/BourgeoisBear/range2cidr"
	"go.etcd.io/bbolt"
)

type CmdExec interface {
	Exec(CmdExecParams) error
}

type CmdExecParams struct {
	Modes
	Db        *bbolt.DB
	Cmd       string
	MaxCmdLen int
}

type RowWriters struct {
	WriteASN ColWriterFunc
	WriteIP  ColWriterFunc
}

func (cep CmdExecParams) getRowWriters() RowWriters {

	writerCfg := ColWriterCfg{Spacer: "|", Pad: cep.Pretty}

	ccfgASN := []ColCfg{
		ColCfg{Wid: 9, Title: "RIR"},
		ColCfg{Wid: 3, Title: "CC"},
		ColCfg{Wid: 4, Title: "TYPE"},
		ColCfg{Wid: 10, Title: "FROM", Rt: true},
		ColCfg{Wid: 10, Title: "TO", Rt: true},
		ColCfg{Wid: 10, Title: "DATE"},
		ColCfg{Wid: 10, Title: "STS"},
		ColCfg{Title: "NAME"},
	}

	ccfgIP := []ColCfg{
		ColCfg{Wid: 9, Title: "RIR"},
		ColCfg{Wid: 3, Title: "CC"},
		ColCfg{Wid: 4, Title: "TYPE"},
		ColCfg{Wid: 23, Title: "SUBNET", Rt: true},
		ColCfg{Wid: 10, Title: "DATE"},
		ColCfg{Wid: 10, Title: "STS"},
	}

	if cep.PrependQuery {
		ccQuery := ColCfg{Wid: cep.MaxCmdLen, Title: "QRY"}
		ccfgASN = append([]ColCfg{ccQuery}, ccfgASN...)
		ccfgIP = append([]ColCfg{ccQuery}, ccfgIP...)
	}

	return RowWriters{
		WriteASN: writerCfg.NewWriterFunc(os.Stdout, ccfgASN),
		WriteIP:  writerCfg.NewWriterFunc(os.Stdout, ccfgIP),
	}
}

func (cep CmdExecParams) printRow(rw RowWriters, pR *Row) error {

	if pR == nil {
		return nil
	}

	fmtDate := func(in []byte) []byte {
		if !cep.Pretty || (len(in) < 8) {
			return in
		}
		return bytes.Join([][]byte{in[:4], in[4:6], in[6:]}, []byte{'-'})
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

		sFields := make([]interface{}, 0, 9)
		if cep.PrependQuery {
			sFields = append(sFields, cep.Cmd)
		}

		sFields = append(sFields,
			pR.Registry,
			pR.Cc,
			pR.Type,
			szAsnFirst,
			szAsnLast,
			fmtDate(pR.Date),
			pR.Status,
			pR.AsName,
		)

		_, err := rw.WriteASN(sFields...)
		return err
	}

	// repeat for each subnet
	for _, r := range pR.IpRange {

		if !r.IsValid() {
			continue
		}

		sFields := make([]interface{}, 0, 7)
		if cep.PrependQuery {
			sFields = append(sFields, cep.Cmd)
		}

		sFields = append(sFields,
			pR.Registry,
			pR.Cc,
			pR.Type,
			r.String(),
			fmtDate(pR.Date),
			pR.Status,
		)

		_, err := rw.WriteIP(sFields...)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cep CmdExecParams) printRowsSorted(rw RowWriters, sRows []Row) error {

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

		sASN[ix].AsName, err = AsnToName(cep.Db, sASN[ix].ASN)
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
			if err := cep.printRow(rw, pRow); err != nil {
				return err
			}
		}
	}

	return nil
}

func (cep CmdExecParams) printRowAssoc(rw RowWriters, pR *Row, bAssoc bool) error {

	sRows := []Row{*pR}
	if bAssoc {
		var err error
		sRows, err = FindAssociated(cep.Db, pR.Registry, pR.RegId)
		if err != nil {
			return err
		}
	}
	return cep.printRowsSorted(rw, sRows)
}

func (v CmdIP) Exec(cep CmdExecParams) error {

	if row, err := IpToRow(cep.Db, v.IP); err != nil {
		return err
	} else {
		return cep.printRowAssoc(cep.getRowWriters(), &row, v.Assoc)
	}
}

func (v CmdASN) Exec(cep CmdExecParams) error {

	if row, err := AsnToRow(cep.Db, v.ASN); err != nil {
		return err
	} else {
		return cep.printRowAssoc(cep.getRowWriters(), &row, v.Assoc)
	}
}

func (v CmdAsName) Exec(cep CmdExecParams) error {

	sRows, err := NameRegexToASNs(cep.Db, v.Name)
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
			sTmp, err := FindAssociated(cep.Db, pr.Registry, pr.RegId)
			if err != nil {
				return err
			}
			sRows = append(sRows, sTmp...)
		}
	}

	// print collection
	return cep.printRowsSorted(cep.getRowWriters(), sRows)
}

/*
	TODO:
		- update documentation for commands
		- headers option
		- unit tests
		- embed parse regexs in command type
*/

func (v CmdRDAP_IP) Exec(cep CmdExecParams) error {

	bsJSON, err := rdap.QueryByIP(v.RIR, v.IP)
	if err != nil {
		return err
	}
	return cep.PrintJSON(os.Stdout, bsJSON)
}

func (v CmdRDAP_Org) Exec(cep CmdExecParams) error {

	bsJSON, err := rdap.QueryByOrg(v.RIR, v.OrgId)
	if err != nil {
		return err
	}

	if !v.NetsOnly {
		return cep.PrintJSON(os.Stdout, bsJSON)
	}

	var ent rdap.Entity
	err = json.Unmarshal(bsJSON, &ent)
	if err != nil {
		os.Stderr.Write(bsJSON)
		return err
	}

	writerCfg := ColWriterCfg{Spacer: "|", Pad: cep.Pretty}
	ccfg := []ColCfg{
		ColCfg{Wid: 9},
		ColCfg{Wid: 4},
		ColCfg{Wid: 23, Rt: true},
		ColCfg{Wid: 10},
		ColCfg{Wid: 10},
		ColCfg{Wid: 10},
	}
	if cep.PrependQuery {
		ccfg = append([]ColCfg{ColCfg{Wid: cep.MaxCmdLen}}, ccfg...)
	}
	fnWri := writerCfg.NewWriterFunc(os.Stdout, ccfg)

	for _, ipnet := range ent.Networks {

		A, err := netip.ParseAddr(ipnet.StartAddress)
		if err != nil {
			cep.printErr(fmt.Errorf("parse start addr: %w", err), cep.Cmd)
		}
		B, err := netip.ParseAddr(ipnet.EndAddress)
		if err != nil {
			cep.printErr(fmt.Errorf("parse end addr: %w", err), cep.Cmd)
		}
		sPfx, err := range2cidr.Deaggregate(A, B)
		if err != nil {
			cep.printErr(fmt.Errorf("deaggregate: %w", err), cep.Cmd)
		}

		for _, pfx := range sPfx {
			addrVer := "IPV4"
			if pfx.Addr().Is6() {
				addrVer = "IPV6"
			}
			parts := []interface{}{
				v.RIR.String(),
				addrVer,
				pfx.String(),
				"-",
				"-",
				strings.ToUpper(strings.Join(ipnet.Status, ":")),
			}

			for _, evt := range ipnet.Events {
				switch evt.Action {
				case "registration":
					parts[3], _, _ = strings.Cut(evt.Date, "T")
				case "last changed":
					parts[4], _, _ = strings.Cut(evt.Date, "T")
				}
			}
			_, err := fnWri(parts...)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (v CmdCC) Exec(cep CmdExecParams) error {

	rw := cep.getRowWriters()
	bsCC := []byte("|" + strings.ToUpper(v.CC) + "|")
	nFound := 0
	err := WalkRawRows(cep.Db, func(_, bsData []byte) error {
		if !bytes.Contains(bsData, bsCC) {
			return nil
		}
		if row, e2 := ParseRow(bsData); e2 != nil {
			return e2
		} else {
			nFound += 1
			return cep.printRow(rw, &row)
		}
	})
	if err != nil {
		return err
	}
	if nFound == 0 {
		return ENotFound
	}
	return nil
}

func (v CmdEmail) Exec(cep CmdExecParams) error {

	bsJSON, err := rdapByIp(cep.Db, v.IP)
	if err != nil {
		return err
	}

	var ent rdap.Entity
	err = json.Unmarshal(bsJSON, &ent)
	if err != nil {
		os.Stderr.Write(bsJSON)
		return err
	}

	writerCfg := ColWriterCfg{Spacer: "@@", Pad: cep.Pretty}
	ccfg := []ColCfg{
		ColCfg{Wid: 16},
		ColCfg{Wid: 16},
		ColCfg{},
	}
	if cep.PrependQuery {
		ccfg = append([]ColCfg{ColCfg{Wid: cep.MaxCmdLen}}, ccfg...)
	}
	fnW := writerCfg.NewWriterFunc(os.Stdout, ccfg)

	for _, em := range ent.GetEmailAddrs() {
		var err error
		if cep.PrependQuery {
			_, err = fnW(cep.Cmd, em.Role, em.Handle, em.Addr)
		} else {
			_, err = fnW(em.Role, em.Handle, em.Addr)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (v CmdAll) Exec(cep CmdExecParams) error {

	rw := cep.getRowWriters()
	return WalkRawRows(cep.Db, func(_, bsData []byte) error {
		if row, e2 := ParseRow(bsData); e2 != nil {
			return e2
		} else {
			return cep.printRow(rw, &row)
		}
	})
}
