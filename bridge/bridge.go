package bridge

import (
	"sync"
	"time"

	"example.com/svcmgr/lib/conn"
	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/lib/logs"
)

var (
	ServerTcpEnable  = false
	ServerKcpEnable  = false
	ServerQuicEnable = false
	ServerTlsEnable  = false
	ServerWsEnable   = false
	ServerWssEnable  = false
	ServerSecureMode = false
)

var bridgeHandshakeReadTimeout time.Duration = 10

type Bridge struct {
	Client             *sync.Map
	Register           *sync.Map
	VirtualTcpListener *conn.VirtualListener
	VirtualTlsListener *conn.VirtualListener
	VirtualWsListener  *conn.VirtualListener
	VirtualWssListener *conn.VirtualListener
	OpenHost           chan *file.Host
	OpenTask           chan *file.Tunnel
	CloseTask          chan *file.Tunnel
	CloseClient        chan int
	SecretChan         chan *conn.Secret
	ipVerify           bool
	runList            *sync.Map //map[int]interface{}
	disconnectTime     int
}

func NewTunnel(ipVerify bool, runList *sync.Map, disconnectTime int) *Bridge {
	return &Bridge{
		Client:         &sync.Map{},
		Register:       &sync.Map{},
		OpenHost:       make(chan *file.Host, 100),
		OpenTask:       make(chan *file.Tunnel, 100),
		CloseTask:      make(chan *file.Tunnel, 100),
		CloseClient:    make(chan int, 100),
		SecretChan:     make(chan *conn.Secret, 100),
		ipVerify:       ipVerify,
		runList:        runList,
		disconnectTime: disconnectTime,
	}
}

func (s *Bridge) DelClient(id int) {
	if v, ok := s.Client.Load(id); ok {
		client := v.(*Client)
		_ = client.Close()

		s.Client.Delete(id)

		if file.GetDb().IsPubClient(id) {
			return
		}
		if c, err := file.GetDb().GetClient(id); err == nil {
			select {
			case s.CloseClient <- c.Id:
			default:
				logs.Warn("CloseClient channel is full, failed to send close signal for client %d", c.Id)
			}
		}
	}
}

func (s *Bridge) IsServer() bool {
	return true
}
