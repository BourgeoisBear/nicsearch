package rdap

import (
	"fmt"
	"io"
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

func (k RIRKey) String() string {
	switch k {
	case RkAfrinic:
		return "AFRINIC"
	case RkApnic:
		return "APNIC"
	case RkArin:
		return "ARIN"
	case RkLacnic:
		return "LACNIC"
	case RkRipe:
		return "RIPENCC"
	}
	return "UNKNOWN"
}

func RegistryNameToKey(regName string) (RIRKey, error) {

	regName = strings.ToUpper(strings.TrimSpace(regName))
	switch regName {
	case "AFRINIC":
		return RkAfrinic, nil
	case "APNIC":
		return RkApnic, nil
	case "ARIN":
		return RkArin, nil
	case "LACNIC":
		return RkLacnic, nil
	case "RIPENCC":
		return RkRipe, nil
	}
	return RkMAX, fmt.Errorf("'%s' is not a valid registry name.  Valid registry names are: AFRINIC, APNIC, ARIN, LACNIC, and RIPENCC.", regName)
}

func getUrl(url string) ([]byte, error) {

	rsp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	// error non non-200
	if rsp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", rsp.Status, url)
	}

	// read response
	return io.ReadAll(rsp.Body)
}

func getRIRUrl(key RIRKey, url string) ([]byte, error) {
	mUrl := GetRDAPUrls()
	return getUrl(mUrl[key] + url)
}

func QueryByOrg(key RIRKey, orgId string) ([]byte, error) {
	return getRIRUrl(key, "/entity/"+orgId)
}

func QueryByIP(key RIRKey, ip netip.Addr) ([]byte, error) {
	return getRIRUrl(key, "/ip/"+ip.String())
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
