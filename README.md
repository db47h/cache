# lrucache

[![Build Status][ci-img]][ci] [![Go Report Card][lint-img]][lint] [![Coverage Status][cover-img]][cover] [![GoDoc][godoc-img]][godoc]

Package lrucache implements a LRU cache with fixed maximum size which removes the
least recently used entry if an entry is added when full.

It supports entry removal callbacks and has an atomic Get/Set operation ([GetWithDefault]).

## Installation

```bash
go get -u github.com/db47h/lrucache
```

## Usage

Read the [API docs][godoc].

Some sample code:

TODO


## Concurrent use

TODO

## Specializing the Key and Value types

The Key and Value types are defined in [types.go] as interfaces. Users who need to
use concrete types instead of interfaces can easily customize these by vendoring
the package then redefine Key and Value in types.go. This file is dedicated to
this purpose and should not change in future versions.

[ci]: https://travis-ci.org/db47h/lrucache
[ci-img]: https://travis-ci.org/db47h/lrucache.svg?branch=master
[lint]: https://goreportcard.com/report/github.com/db47h/lrucache
[lint-img]: https://goreportcard.com/badge/github.com/db47h/lrucache
[cover]: https://coveralls.io/github/db47h/lrucache
[cover-img]: https://coveralls.io/repos/github/db47h/lrucache/badge.svg
[godoc]: https://godoc.org/github.com/db47h/lrucache
[godoc-img]: https://godoc.org/github.com/db47h/lrucache?status.svg

[GetWithDefault]: https://godoc.org/github.com/db47h/lrucache#LRUCache.GetWithDefault
[types.go]: https://github.com/db47h/lrumap/blob/master/types.go
