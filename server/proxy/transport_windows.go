//go:build windows
// +build windows

package proxy

import (
	"example.com/svcmgr/lib/conn"
)

func HandleTrans(c *conn.Conn, s *TunnelModeServer) error {
	return nil
}
