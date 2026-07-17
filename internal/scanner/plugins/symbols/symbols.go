// Package nscan provides yaegi symbol tables for nscan internal packages.
package nscan

import "reflect"

// Symbols maps import path to exported symbols for yaegi interpreter use.
var Symbols = map[string]map[string]reflect.Value{}
