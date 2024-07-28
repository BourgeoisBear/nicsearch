module github.com/BourgeoisBear/nicsearch

go 1.22.5

require (
	github.com/BourgeoisBear/nicsearch/colwriter v0.0.0-00010101000000-000000000000
	github.com/BourgeoisBear/nicsearch/rdap v0.0.0-00010101000000-000000000000
	github.com/BourgeoisBear/range2cidr v0.0.1
	github.com/chzyer/readline v1.5.1
	github.com/mattn/go-isatty v0.0.17
	github.com/pkg/errors v0.9.1
	go.etcd.io/bbolt v1.3.7
)

replace github.com/BourgeoisBear/nicsearch/rdap => ./rdap

replace github.com/BourgeoisBear/nicsearch/colwriter => ./colwriter

require golang.org/x/sys v0.4.0 // indirect
