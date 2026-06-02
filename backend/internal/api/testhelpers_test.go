package api

import "strconv"

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
