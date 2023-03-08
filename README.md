# nicsearch
Offline lookup (by IP/ASN) of other IPs/ASNs owned by the same organization.  It can also dump IPs/ASNs by country code and map most ASNs to their names.  Uses locally cached data, downloaded from all regional internet registries (RIRs).  This prevents throttlings and timeouts on high-volume lookups of IPs/ASNs.

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

```sh
go install github.com/BourgeoisBear/nicsearch@latest
```

## Usage

If no `[QUERY]` items are supplied to the command line, nicsearch opens in interactive mode.  In this mode, the user can supply individual queries, each followed by the `<Enter>` key.  RIR data is automatically downloaded and indexed on first invocation.  By default, `nicsearch` caches RIR data in `$HOME/.cache/nicsearch` as gzipped text files, but this location can be overridden with the `-dbpath` flag.

```
Usage of nicsearch:

USAGE: nicsearch [OPTION]... [QUERY]...

OPTION
  -color
    force color output on/off (-color=t vs -color=f)
  -dbpath string
    override path to RIR data and index
  -download
    force download of RIR databases
  -pretty
    force pretty print on/off (-pretty=t vs -pretty=f)
  -reindex
    foce rebuild of RIR database index
        
QUERY
  AS[0-9]+
    query by autonomous system number (ASN)
    example: AS123456

    add the suffix ,a to get all IPs and ASNs with the same owner
    example: AS123456,a
  	
  172.104.6.84
  2620:118:7000::/44
    query by IP (v4 or v6) address
    example: 172.104.6.84
    
    add the suffix ,a to get all IPs and ASNs with the same owner
    example: 172.104.6.84,a
        
  CC[A-Z]{2}
    query by country code.  returns all IPs & ASNs for the given country.
    example: CCUS
      
```
