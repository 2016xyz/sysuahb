export GOPROXY=direct

NPS_BIN="sysuahb"
NPC_BIN="sysficb"

sudo apt-get update
sudo apt-get install gcc-mingw-w64-i686 gcc-multilib
env GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc go build -tags sdk -ldflags "-s -w -extldflags -static -extldflags -static" -buildmode=c-shared -o ${NPC_BIN}_sdk.dll cmd/npc/sdk.go
env GOOS=linux GOARCH=386 CGO_ENABLED=1 CC=gcc go build -tags sdk -ldflags "-s -w -extldflags -static -extldflags -static" -buildmode=c-shared -o ${NPC_BIN}_sdk.so cmd/npc/sdk.go
tar -czvf ${NPC_BIN}_sdk_old.tar.gz ${NPC_BIN}_sdk.dll ${NPC_BIN}_sdk.so npc_sdk.h

CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'example.com/svcmgr/lib/install.BuildTarget=win7'" -o ${NPC_BIN}.exe ./cmd/npc/client.go

tar -czvf windows_386_client_old.tar.gz ${NPC_BIN}.exe conf/sysficb.conf conf/multi_account.conf


CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'example.com/svcmgr/lib/install.BuildTarget=win7'" -o ${NPC_BIN}.exe ./cmd/npc/client.go

tar -czvf windows_amd64_client_old.tar.gz ${NPC_BIN}.exe conf/sysficb.conf conf/multi_account.conf


CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'example.com/svcmgr/lib/install.BuildTarget=win7'" -o ${NPS_BIN}.exe ./cmd/nps/nps.go

tar -czvf windows_amd64_server_old.tar.gz conf/sysuahb.conf web/views web/static ${NPS_BIN}.exe


CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'example.com/svcmgr/lib/install.BuildTarget=win7'" -o ${NPS_BIN}.exe ./cmd/nps/nps.go

tar -czvf windows_386_server_old.tar.gz conf/sysuahb.conf web/views web/static ${NPS_BIN}.exe
