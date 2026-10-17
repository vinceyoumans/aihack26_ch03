package main

import (
	"strconv"
	"strings"
)

// strconvFormat formats a float with the given precision.
func strconvFormat(f float64, prec int) string {
	return strconv.FormatFloat(f, 'f', prec, 64)
}

// trimZeros removes a trailing ".0" so "7.0" renders as "7".
func trimZeros(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
