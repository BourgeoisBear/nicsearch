# nicsearch
Offline lookup (by IP/ASN) of other IPs/ASNs owned by the same organization.  Can also dump IPs/ASNs by country code.  Uses locally cached data downloaded from all regional internet registries (RIRs).

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

```sh
go install github.com/BourgeoisBear/nicsearch@latest
```

## Usage

If no `[QUERY]` items are supplied to the command line, nicsearch opens in interactive mode where the user can supply individual queries&mdash;each followed by the `<Enter>` key.  RIR data will be automatically downloaded and indexed on first run.  It caches RIR data in `$HOME/.cache/nicsearch` as gzipped text files.

```
Usage of nicsearch:

USAGE: nicsearch [OPTION]... [QUERY]...

OPTIONS
  -color
    force color output on/off (default true)
  -dbpath string
    path to RIR data and index (default "/home/jstewart/.cache/nicsearch")
  -download
    download RIR databases
  -pretty
    force pretty print on/off (default true)
  -reindex
    rebuild RIR database index
        
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
