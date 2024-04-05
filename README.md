# nicsearch

Offline lookup by IP/ASN of other IPs/ASNs belonging to the same organization. This tool can also dump IPs/ASNs by country code, as well as map most ASNs to their names.  Uses locally cached data, downloaded from all regional internet registries (RIRs) to prevent throttlings and timeouts on high-volume lookups.

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

```sh
go install github.com/BourgeoisBear/nicsearch@latest
```

## Usage

Can be used in shell pipelines by providing `[QUERY]` items as arguments:
```
nicsearch 'ip 172.104.6.84 +' | grep 'ASN'
```

Or, invoke without `[QUERY]` items for interactive mode:
```
nicsearch
```

In this mode, the user can supply individual queries inside a REPL environment.  RIR data is automatically downloaded and indexed on first invocation.  By default, `nicsearch` caches RIR data in `$HOME/.cache/nicsearch` as gzipped text files, but this location can be overridden with the `-dbpath` flag.

```
USAGE
  nicsearch [OPTION]... [QUERY]...

    Offline lookup by IP/ASN of other IPs/ASNs owned by the same organization.
    This tool can also dump IPs/ASNs by country code, as well as map most ASNs to
    their names.  Uses locally cached data, downloaded from all regional internet
    registries (RIRs) to prevent throttlings and timeouts on high-volume lookups.

OPTION
  -color
    	force color output on/off
  -dbpath string
    	override path to RIR data and index (default "/home/jstewart/.cache/nicsearch")
  -download
    	force download of RIR databases
  -prependQuery
    	prepend query to corresponding result row in tabular outputs
  -pretty
    	force pretty print on/off
  -reindex
    	force rebuild of RIR database index

QUERY
  as ASN [+]
    query by autonomous system number (ASN).
      ex: 'as 14061'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
      ex: 'as 14061 +'

  ip IPADDR [+]
    query by IP (v4 or v6) address.
      ex: 'ip 172.104.6.84'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
      ex: 'ip 172.104.6.84 +'

  cc COUNTRY_CODE
    query by country code.
    returns all IPs & ASNs for the given country.
      ex: 'cc US'

  na REGEX [+]
    query by ASN name.
    returns all ASNs with names matching the given REGEX.
    see https://pkg.go.dev/regexp/syntax for syntax rules.
      ex: 'na microsoft'

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization(s) of all matching ASNs.
      ex: 'na microsoft +'

  rdap.email IPADDR
    get email contacts for IPADDR.
      ex: 'rdap.email 8.8.8.8'

    NOTE: columns are separated by '@@' instead of '|' since pipe can
          appear inside the unquoted local-part of an email address.

  rdap.ip RIR IPADDR
    get full RDAP reply (in JSON) from RIR for IP address.
      ex: 'rdap.ip arin 8.8.8.8'

  rdap.org RIR ORGID
    get full RDAP reply (in JSON) from RIR for ORGID.
      ex: 'rdap.org arin DO-13'

  rdap.orgnets RIR ORGID
    an 'rdap.org' query, returning only the associated IP networks
    section in table format.
      ex: 'rdap.orgnets arin DO-13'

  all
    dump all local records

  NOTE: all 'rdap.' queries require an internet connection to the
        RIR's RDAP service.
```

## RIR Stats Exchange Format

https://www.apnic.net/about-apnic/corporate-documents/documents/resource-guidelines/rir-statistics-exchange-format/

### Fields

```
    registry|cc|type|start|value|date|status[|extensions...]

registry
    One value from the set of defined strings: {afrinic,apnic,arin,iana,lacnic,ripencc};

cc
    ISO 3166 2-letter code of the organization to which the allocation or
    assignment was made. It should be noted that these reports seek to indicate
    where resources were first allocated or assigned. It is not intended that
    these reports be considered as an authoritative statement of the location
    in which any specific resource may currently be in use.

type
    Type of Internet number resource represented in this record. One value from
    the set of defined strings: {asn,ipv4,ipv6}

start
    In the case of records of type ‘ipv4’ or ‘ipv6’ this is the IPv4 or IPv6
    ‘first address’ of the range. In the case of an 16 bit AS number the format
    is the integer value in the range 0 to 65535, in the case of a 32 bit ASN
    the value is in the range 0 to 4294967296. No distinction is drawn between
    16 and 32 bit ASN values in the range 0 to 65535.

value
    In the case of IPv4 address the count of hosts for this range. This count
    does not have to represent a CIDR range.

    In the case of an IPv6 address the value will be the CIDR prefix length
    from the ‘first address’ value of <start>.

    In the case of records of type ‘asn’ the number is the count of AS from
    this start value.

date
    Date on this allocation/assignment was made by the RIR in the format
    YYYYMMDD;

    Where the allocation or assignment has been transferred from another
    registry, this date represents the date of first assignment or allocation
    as received in from the original RIR.

    It is noted that where records do not show a date of first assignment, this
    can take the 0000/00/00 value

status
    Type of allocation from the set: This is the allocation or assignment made
    by the registry producing the file and not any sub-assignment by other
    agencies.

extensions
    Any extra data on a line is undefined, but should conform to use of the
    field separator used above.

```
