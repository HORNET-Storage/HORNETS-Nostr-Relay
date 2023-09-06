cd web/panel
cmd /C npm i
cd ../..
cd services/server
cmd /C go build -o ../../hornet-storage.exe
cd ../client
cmd /C go build -o ../../
pause