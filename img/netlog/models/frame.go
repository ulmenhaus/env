package models

import (
	"strconv"
	"strings"
)

type MACAddress []byte

func (a MACAddress) String() string {
	hexes := []string{}
	for _, b := range a {
		hex := strconv.FormatInt(int64(b), 16)
		if len(hex) == 1 {
			hexes = append(hexes, "0" + hex)
		} else {
			hexes = append(hexes, hex)
		}
	}
	return strings.Join(hexes, ":")
}

// An IEEEFrame models a 802.3 ethernet frame
type IEEEFrame []byte

func (f IEEEFrame) Source() MACAddress {
	return MACAddress(f[6:12])
}

func (f IEEEFrame) Dest() MACAddress {
	return MACAddress(f[0:6])
}
