// TODO: header line in pretty mode
// TODO: sort by ip rather than registry in pretty mode

- ASN name search (API)

	https://ftp.arin.net/pub/stats/ripencc/delegated-ripencc-latest
	https://ftp.ripe.net/ripe/asnames/asn.txt
	https://ftp.arin.net/info/asn.txt
	https://stat.ripe.net/data/as-overview/data.json?resource=AS14061

	https://stat.ripe.net/docs/02.data-api/abuse-contact-finder.html
	https://securitytrails.com/blog/asn-lookup
	https://bgp.potaroo.net/cidr/autnums.html

- unit tests

## RIR stats exchange format

https://www.apnic.net/about-apnic/corporate-documents/documents/resource-guidelines/rir-statistics-exchange-format/

### Format:

    registry|cc|type|start|value|date|status[|extensions...]

### Description

registry
    One value from the set of defined strings:
    {afrinic,apnic,arin,iana,lacnic,ripencc};

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
