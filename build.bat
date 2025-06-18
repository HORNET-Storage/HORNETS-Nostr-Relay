set CGO_ENABLED=1
cd services/server/port
cmd /C go build -o ../../../hornet-storage.exe
pause