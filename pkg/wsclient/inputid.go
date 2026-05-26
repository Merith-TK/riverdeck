package wsclient

import (
	"strconv"
	"strings"
)

func InputIDToIndex(id string) int {
	if !strings.HasPrefix(id, "btn") {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "btn"))
	if err != nil {
		return -1
	}
	return n
}
