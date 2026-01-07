@echo off
set CGO_ENABLED=1
pushd "%~dp0services\server\port"
go build -o "%~dp0hornet-storage.exe"
popd