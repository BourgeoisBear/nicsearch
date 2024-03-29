package rdap

import (
	"bytes"
	"encoding/json"
)

/*
   https://www.apnic.net/manage-ip/using-whois/abuse-and-spamming/reporting-abuse-and-spam/

   NIR    Nation     Whois Database
   -----  --------   ----------
   APJII  Indonesia  Refer to APNIC Whois Database
   CNNIC  China      Refer to APNIC Whois Database
   JPNIC  Japan      http://whois.nic.ad.jp/cgi-bin/whois_gw
   KRNIC  Korea      http://whois.nic.or.kr/english/
   TWNIC  Taiwan     http://www.twnic.net/index2.php
   VNNIC  VietNam    Refer to APNIC Whois Database
   IRINN  India      Refer to https://irinn.in/

   https://rdap.lacnic.net/rdap/ip/45.226.132.0
   https://rdap.arin.net/registry/ip/8.8.8.8
   https://rdap.apnic.net/ip/118.32.0.1
   https://rdap.db.ripe.net/ip/{key}
   https://rdap.afrinic.net/rdap/ip/212.103.160.0
*/

type Common struct {
	Lang string
}

// Link signifies a link another resource on the Internet.
// https://tools.ietf.org/html/rfc7483#section-4.2
type Link struct {
	Value    string
	Rel      string
	Href     string
	HrefLang OnePlusString `json:"hreflang"`
	Title    string
	Media    string
	Type     string
}

// Notice contains information about the entire RDAP response.
// https://tools.ietf.org/html/rfc7483#section-4.3
type Notice struct {
	Title       string
	Type        string
	Description OnePlusString
	Links       []Link
}

// Remark contains information about the containing RDAP object.
// https://tools.ietf.org/html/rfc7483#section-4.3
type Remark struct {
	Title       string
	Type        string
	Description OnePlusString
	Links       []Link
}

// Event represents some event which has occured/may occur in the future..
// https://tools.ietf.org/html/rfc7483#section-4.5
type Event struct {
	Action string        `json:"eventAction"`
	Actor  string        `json:"eventActor"`
	Date   string        `json:"eventDate"`
	Status OnePlusString `json:"status"`
	Links  []Link
}

// PublicID maps a public identifier to an object class.
// https://tools.ietf.org/html/rfc7483#section-4.8
type PublicID struct {
	Type       string
	Identifier string
}

type OnePlusString []string

func (pv *OnePlusString) UnmarshalJSON(bs []byte) error {

	*pv = make([]string, 0)

	if len(bs) == 0 {
		return nil
	}

	iRd := bytes.NewReader(bs)
	pDec := json.NewDecoder(iRd)

	for pDec.More() {
		tok, err := pDec.Token()
		if err != nil {
			return err
		}

		switch v := tok.(type) {
		case string:
			*pv = append(*pv, v)
		}
	}

	return nil
}

// IPNetwork represents information of an IP Network.
// IPNetwork is a topmost RDAP response object.
type IPNetwork struct {
	Common
	Conformance     OnePlusString `json:"rdapConformance"`
	ObjectClassName string
	Notices         []Notice

	Handle       string
	StartAddress string
	EndAddress   string
	IPVersion    string `json:"ipVersion"`
	Name         string
	Type         string
	Country      string
	ParentHandle string
	Status       OnePlusString
	Entities     []Entity
	Remarks      []Remark
	Links        []Link
	Port43       string
	Events       []Event
}

// Autnum represents information of Autonomous System registrations.
// Autnum is a topmost RDAP response object.
type Autnum struct {
	Common
	Conformance     OnePlusString `json:"rdapConformance"`
	ObjectClassName string
	Notices         []Notice

	Handle      string
	StartAutnum *uint32
	EndAutnum   *uint32
	IPVersion   string `json:"ipVersion"`
	Name        string
	Type        string
	Status      OnePlusString
	Country     string
	Entities    []Entity
	Remarks     []Remark
	Links       []Link
	Port43      string
	Events      []Event
}

// Entity is a topmost RDAP response object.
type Entity struct {
	Common
	Conformance     OnePlusString `json:"rdapConformance"`
	ObjectClassName string
	Notices         []Notice

	Handle       string
	VCard        VCard `json:"vcardArray"`
	Roles        OnePlusString
	PublicIDs    []PublicID `json:"publicIds"`
	Entities     []Entity
	Remarks      []Remark
	Links        []Link
	Events       []Event
	AsEventActor []Event
	Status       OnePlusString
	Port43       string
	Networks     []IPNetwork
	Autnums      []Autnum
}
