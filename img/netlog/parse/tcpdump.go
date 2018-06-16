package parse

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ulmenhaus/env/img/netlog/models"
)

type EthernetEvent struct {
	Frame models.IEEEFrame
}

func ParseTCPDumpHexDump(b []byte) ([]byte, error) {
	bytes := []byte{}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if len(line) < 56 {
			continue
		}
		parts := strings.Split(strings.Replace(line[:56], " ", "", -1), ":")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid dump -- no ':'")
		}
		chars := parts[1]
		if (len(chars) % 2) != 0 {
			return nil, fmt.Errorf("invalid dump -- not an even number of chars")
		}
		for i := 0; i < len(chars); i += 2 {
			b, err := strconv.ParseInt(chars[i:i+2], 16, 9)
			if err != nil {
				return nil, err
			}
			bytes = append(bytes, byte(b))
		}
	}
	return bytes, nil
}

func ParseTCPDumpStream(r io.Reader) (<-chan EthernetEvent, <-chan error) {

}
