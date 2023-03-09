# nicsearch
Offline lookup by IP/ASN of other IPs/ASNs belonging to the same organization. This tool can also dump IPs/ASNs by country code, as well as map most ASNs to their names.  Uses locally cached data, downloaded from all regional internet registries (RIRs) to prevent throttlings and timeouts on high-volume lookups.

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

```sh
go install github.com/BourgeoisBear/nicsearch@latest
```

## Usage

If no `[QUERY]` items are supplied to the command line, nicsearch opens in interactive mode.  In this mode, the user can supply individual queries, each followed by the `<Enter>` key.  RIR data is automatically downloaded and indexed on first invocation.  By default, `nicsearch` caches RIR data in `$HOME/.cache/nicsearch` as gzipped text files, but this location can be overridden with the `-dbpath` flag.

```
nicsearch [OPTION]... [QUERY]...

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
  as ASN [+]
    query by autonomous system number (ASN).
    example: as 14061

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
    example: as 14061 +

  ip IPADDR [+]
    query by IP (v4 or v6) address.
    example: ip 172.104.6.84

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization.
    example: ip 172.104.6.84 +

  cc COUNTRY_CODE
    query by country code.  returns all IPs & ASNs for the given country.
    example: cc US

  na REGEX [+]
    query by ASN name.  returns all ASNs with names matching the given REGEX.
    see https://pkg.go.dev/regexp/syntax for syntax rules.
    example: na microsoft

    add the suffix '+' to return all IPs and ASNs associated
    by 'reg-id' with the same organization(s) of all matching ASNs.
    example: na microsoft +
```
