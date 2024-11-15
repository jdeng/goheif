//go:build vendorkeep
// +build vendorkeep

package libde265

// https://github.com/golang/go/issues/26366

// This file exists purely to prevent the golang toolchain from stripping
// away the c source directories and files when `go mod vendor` is used
// to populate a `vendor/` directory of a project depending on `goheif`.
//
// How it works:
//  - every directory which only includes c/c++ source files receives a
//    vendorkeep.go file.
//  - every directory we want to preserve is included here as a _ import.
//  - every dummy go file is given a build tag to exclude it from the regular
//    build.

import (
	// Prevent go tooling from stripping out the c source files.
	_ "github.com/jdeng/goheif/libde265/extra"
	_ "github.com/jdeng/goheif/libde265/libde265"
	_ "github.com/jdeng/goheif/libde265/libde265/arm"
	_ "github.com/jdeng/goheif/libde265/libde265/x86"
)
