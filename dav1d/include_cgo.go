//go:build vendorkeep
// +build vendorkeep

package dav1d

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
	_ "github.com/jdeng/goheif/dav1d/include/common"
	_ "github.com/jdeng/goheif/dav1d/include/compat"
	_ "github.com/jdeng/goheif/dav1d/include/compat/gcc"
	_ "github.com/jdeng/goheif/dav1d/include/compat/msvc"
	_ "github.com/jdeng/goheif/dav1d/include/dav1d"
	_ "github.com/jdeng/goheif/dav1d/src"
	_ "github.com/jdeng/goheif/dav1d/src/arm"
	_ "github.com/jdeng/goheif/dav1d/src/arm/32"
	_ "github.com/jdeng/goheif/dav1d/src/arm/64"
	_ "github.com/jdeng/goheif/dav1d/src/loongarch"
	_ "github.com/jdeng/goheif/dav1d/src/ppc"
	_ "github.com/jdeng/goheif/dav1d/src/riscv"
	_ "github.com/jdeng/goheif/dav1d/src/riscv/64"
	_ "github.com/jdeng/goheif/dav1d/src/win32"
	_ "github.com/jdeng/goheif/dav1d/src/x86"
)
