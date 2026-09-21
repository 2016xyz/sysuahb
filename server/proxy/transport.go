//go:build !windows
// +build !windows

package proxy

import (
	"example.com/svcmgr/lib/common"
	"example.com/svcmgr/lib/conn"
	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/lib/transport"
)

func HandleTrans(c *conn.Conn, s *TunnelModeServer) error {
	if addr, err := transport.GetAddress(c.Conn); err != nil {
		return err
	} else {
		return s.DealClient(c, s.Task.Client, addr, nil, common.CONN_TCP, nil, []*file.Flow{s.Task.Flow, s.Task.Client.Flow}, s.Task.Target.ProxyProtocol, s.Task.Target.LocalProxy, s.Task)
	}
}
