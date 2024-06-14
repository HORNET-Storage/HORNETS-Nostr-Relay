cd services/server
cmd /C go build -o ../../hornet-storage.exe
cd port
cmd /C go build -o ../../../hornet-storage-port.exe
pause