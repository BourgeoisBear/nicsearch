#!/bin/sh

while read -r ITEM
do
	DOMAIN="${ITEM%% *}"
	DIR="${ITEM##* }"
	#URL="http://$DOMAIN/pub/stats/$DIR/transfers/transfers_latest.json"
	URL="http://$DOMAIN/pub/stats/$DIR/delegated-${DIR}-extended-latest"
	wget --show-progress -O "${DIR}_latest.txt" "$URL"
done << EOF
ftp.ripe.net ripencc
ftp.lacnic.net lacnic
ftp.afrinic.net afrinic
ftp.apnic.net apnic
ftp.arin.net arin
EOF

exit 0

# TODO: lookup ip, get reg-id, get other ips from reg-id
# TODO: filter by: country, asn|ip4+6|ip4|ip6, common owner (reg-id)

# HAS MEMBERSHIP: ripe,
# 'http://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest'
# 'http://ftp.lacnic.net/pub/stats/lacnic/delegated-lacnic-latest'
# 'http://ftp.afrinic.net/pub/stats/afrinic/delegated-afrinic-latest'
# 'http://ftp.apnic.net/pub/stats/apnic/delegated-apnic-latest'
# 'http://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest'

