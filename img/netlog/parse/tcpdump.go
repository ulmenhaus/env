package parse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/ulmenhaus/env/img/netlog/models"
)

type EthernetEvent struct {
	Frame models.IEEEFrame
}

type TCPDumpParser struct {
	prefixExpr *regexp.Regexp
	dumpExpr   *regexp.Regexp
	lenExpr    *regexp.Regexp
}

func NewTCPDumpParser() (*TCPDumpParser, error) {
	prefixExpr, err := regexp.Compile("^[0-9][0-9]:[0-9][0-9]:[0-9][0-9].*length ")
	if err != nil {
		return nil, err
	}
	dumpExpr, err := regexp.Compile("^\t0x")
	if err != nil {
		return nil, err
	}
	return &TCPDumpParser{
		prefixExpr: prefixExpr,
		dumpExpr:   dumpExpr,
	}, nil
}

func (p *TCPDumpParser) ParseHexDump(b []byte) ([]byte, error) {
	bytes := []byte{}
	if b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if len(line) < 50 {
			return nil, fmt.Errorf("invalid hex dump line -- fewer than 50 chars")
		}
		chars := strings.Replace(line[10:50], " ", "", -1)
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

func (p *TCPDumpParser) parseStream(r io.Reader, eventChan chan<- EthernetEvent, errChan chan<- error) {
	b := bufio.NewReader(r)
	current := []byte{}
	var err error
	defer func() {
		close(eventChan)
		errChan <- err
		close(errChan)
	}()
	for {
		current, err = b.ReadBytes('\n')
		if err != nil {
			return
		}
		if p.prefixExpr.Match(current) {
			break
		}
	}
	for {
		length := 0
		parts := bytes.Split(current, []byte("length "))
		if len(parts) != 2 {
			err = fmt.Errorf("Could not parse prefix line. Had %d parts.", len(parts))
			err = fmt.Errorf("%s %s", err, current)
			return
		}
		for _, digit := range parts[1] {
			if '0' <= digit && digit <= '9' {
				length = (length * 10) + int(digit-'0')
			} else {
				break
			}
		}
		// XXX does not handle 802.1Q headers
		expected := (length / 16) + 1
		if (length % 16) != 0 {
			expected += 1
		}
		hexDump := []byte{}
		actual := 0
		for {
			current, err = b.ReadBytes('\n')
			if p.dumpExpr.Match(current) {
				actual += 1
				hexDump = append(hexDump, current...)
			}
			if actual == expected {
				var parsed []byte
				parsed, err = p.ParseHexDump(hexDump)
				if err != nil {
					return
				}
				eventChan <- EthernetEvent{
					Frame: models.IEEEFrame(parsed),
				}
				break
			}
		}
		current, err = b.ReadBytes('\n')
		if err != nil {
			return
		}
		if !p.prefixExpr.Match(current) {
			err = fmt.Errorf("expected prefix line after dump")
			return
		}
	}
}

func (p *TCPDumpParser) ParseStream(r io.Reader) (<-chan EthernetEvent, <-chan error) {
	eventChan := make(chan EthernetEvent)
	errChan := make(chan error)
	go p.parseStream(r, eventChan, errChan)
	return eventChan, errChan
}
