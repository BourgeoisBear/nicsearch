package rdap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

type RIRKey int

const (
	RkRipe RIRKey = iota
	RkLacnic
	RkAfrinic
	RkApnic
	RkArin
	RkMAX
)

func GetRDAPUrls() map[RIRKey]string {
	return map[RIRKey]string{
		RkLacnic:  "https://rdap.lacnic.net/rdap",
		RkArin:    "https://rdap.arin.net/registry",
		RkApnic:   "https://rdap.apnic.net",
		RkRipe:    "https://rdap.db.ripe.net",
		RkAfrinic: "https://rdap.afrinic.net/rdap",
	}
}

func RegistryNameToKey(regName string) RIRKey {

	switch regName {

	case "LACNIC":
		return RkLacnic
	case "ARIN":
		return RkArin
	case "APNIC":
		return RkApnic
	case "RIPENCC":
		return RkRipe
	case "AFRINIC":
		return RkAfrinic
	default:
		return RkMAX
	}
}

func QueryRDAPByIP(key RIRKey, ip netip.Addr) (Entity, error) {

	var sE Entity
	mUrl := GetRDAPUrls()
	szUrl := mUrl[key] + "/ip/" + ip.String()

	// request list
	rsp, err := http.Get(szUrl)
	if err != nil {
		return sE, err
	}
	defer rsp.Body.Close()

	// error non non-200
	if rsp.StatusCode != 200 {
		return sE, fmt.Errorf("%s: %s", rsp.Status, szUrl)
	}

	pDec := json.NewDecoder(rsp.Body)
	err = pDec.Decode(&sE)
	return sE, err
}

type EntityEmail struct {
	Role, Handle, Addr string
}

func (e Entity) GetEmailAddrs() []EntityEmail {

	var sEml []EntityEmail

	processEntities(e.Entities, func(ix int, ent Entity) bool {

		// TODO: ISP name, address, telephone might be nice
		for _, vc := range ent.VCard {

			if vc.Name == "email" {

				for _, iV := range vc.Values {

					eml := iV.(string)
					if len(eml) > 0 {

						for _, role := range ent.Roles {
							sEml = append(sEml, EntityEmail{Handle: strings.ToUpper(ent.Handle), Role: strings.ToLower(role), Addr: eml})
						}
					}
				}
			}
		}

		return true
	})

	return sEml
}

func processEntities(sEnt []Entity, fn func(int, Entity) bool) bool {

	for ie := range sEnt {

		ent := sEnt[ie]

		// recurse into sub-entities
		if !processEntities(ent.Entities, fn) {
			return false
		}

		if !fn(ie, sEnt[ie]) {
			return false
		}
	}

	return true
}
