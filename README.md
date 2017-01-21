# lrucache

[![Build Status](https://travis-ci.org/db47h/lrucache.svg?branch=master)](https://travis-ci.org/db47h/lrucache)
[![Go Report Card](https://goreportcard.com/badge/github.com/db47h/lrucache)](https://goreportcard.com/report/github.com/db47h/lrucache)
[![Coverage Status](https://coveralls.io/repos/github/db47h/lrucache/badge.svg)](https://coveralls.io/github/db47h/lrucache)  [![GoDoc](https://godoc.org/github.com/db47h/lrucache?status.svg)](https://godoc.org/github.com/db47h/lrucache)

Package lrucache implements a LRU cache with fixed maximum size which removes the
least recently used entry if an entry is added when full.

It supports entry removal callbacks and has an atomic Get/Set operation (`GetWithDefault`).

## Intsallation

```bash
go get -u github.com/db47h/lrucache
```

## Usage

Check the [![GoDoc](https://godoc.org/github.com/db47h/lrucache?status.svg)](https://godoc.org/github.com/db47h/lrucache)

Some sample code:

TODO


## Concurrent use

TODO

## Specializing the Key and Value types

The Key and Value types are defined in types.go as interfaces. Users who need to
use concrete types instead of interfaces can easily customize these by vendoring
the package then redefine Key and Value in types.go. This file is dedicated to
this purpose and should not change in future versions.



[types.go]: https://github.com/db47h/lrumap/blob/master/types.go
