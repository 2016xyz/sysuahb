package proxy

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
)

var errBadPort = errors.New("invalid port")

// trojanRequest builds the Trojan TCP request:
//
//	hex(sha224(password)) CR LF
//	command(0x01) address-type address port
//	CR LF
func trojanRequest(password, target string) []byte {
	sum := sha256.Sum224([]byte(password))
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		portStr = "443"
	}
	port, _ := parsePort(portStr)

	buf := make([]byte, 0, 128)
	buf = append(buf, hex.EncodeToString(sum[:])...)
	buf = append(buf, '\r', '\n')
	buf = append(buf, 0x01) // CONNECT
	buf = appendTrojanAddr(buf, host)
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], uint16(port))
	buf = append(buf, p[:]...)
	buf = append(buf, '\r', '\n')
	return buf
}

func appendTrojanAddr(buf []byte, host string) []byte {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			buf = append(buf, 0x01)
			return append(buf, v4...)
		}
		buf = append(buf, 0x04)
		return append(buf, ip.To16()...)
	}
	buf = append(buf, 0x03, byte(len(host)))
	return append(buf, host...)
}

func parsePort(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadPort
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 || n > 65535 {
		return 0, errBadPort
	}
	return n, nil
}
