@echo off
setlocal enabledelayedexpansion

echo Starting build and deployment of HORNETS-Relay-Panel...
echo.

REM Step 1: Remove old panel source
echo Removing old panel source...
rmdir /S /Q panel-source 2>nul

REM Step 2: Clone latest panel source from GitHub
echo Cloning latest panel source from GitHub...
git clone https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git panel-source
if not exist panel-source (
    echo Error: Clone failed. panel-source directory not found!
    goto end
)

REM Step 3: Navigate into panel-source for all remaining commands
pushd panel-source

REM Step 4: Install dependencies
echo Installing dependencies with yarn...
call yarn install || echo Warning: Yarn install may have failed.

REM Step 5: Run the panel's own build script
echo Running build.bat inside panel-source...
call build.bat
if errorlevel 1 (
    echo Error: Panel build.bat failed.
    popd
    goto end
)

REM Step 6: Return to root and copy build output
popd
echo Copying build files to web directory...
rmdir /S /Q web 2>nul
mkdir web
xcopy /E /I /Y panel-source\build\* web\ || echo Warning: Copy operation may have failed.

REM Final message
echo.
echo Build and deployment process complete.
echo You can now access the panel at your relay's root URL.
echo.
echo To test the panel:
echo 1. Start your relay server: go run services\server\port\main.go
echo 2. Visit http://localhost:9002 (or your configured port)
echo 3. The panel should load automatically

:end
pause
endlocal
