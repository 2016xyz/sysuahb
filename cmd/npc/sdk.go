//go:build sdk
// +build sdk

package main

import (
	"C"
	"context"

	"example.com/svcmgr/client"
	"example.com/svcmgr/lib/common"
	"example.com/svcmgr/lib/logs"
	"example.com/svcmgr/lib/version"
)

var cl *client.TRPClient

func init() {
	logs.EnableInMemoryBuffer(0) // 0 = Default 64KB
	logs.Init("off", "trace", "", 0, 0, 0, false, false)
}

//export StartClientByVerifyKey
func StartClientByVerifyKey(serverAddr, verifyKey, connType, proxyUrl *C.char) int {
	if cl != nil {
		cl.Close()
	}
	cl = client.NewRPClient(C.GoString(serverAddr), C.GoString(verifyKey), C.GoString(connType), C.GoString(proxyUrl), "", "", nil, 60, nil)
	cl.Start(context.Background())
	return 1
}

//export GetClientStatus
func GetClientStatus() int {
	return client.NowStatus
}

//export CloseClient
func CloseClient() {
	if cl != nil {
		cl.Close()
	}
}

//export Version
func Version() *C.char {
	return C.CString(version.VERSION)
}

//export SetLogsLevel
func SetLogsLevel(logsLevel *C.char) {
	logs.SetLevel(C.GoString(logsLevel))
}

//export Logs
func Logs() *C.char {
	return C.CString(logs.GetBufferedLogs())
}

//export SetDnsServer
func SetDnsServer(dnsServer *C.char) {
	common.SetCustomDNS(C.GoString(dnsServer))
}

func main() {
	// Need a main function to make CGO compile package as C shared library
}
