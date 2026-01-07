@echo off
setlocal EnableExtensions EnableDelayedExpansion

echo Starting build and deployment of HORNETS-Relay-Panel...
echo.

REM --- Config ---
set "CONFIG_FILE=config.yaml"
REM -------------

REM Read base port from config.yaml and calculate web port (+2)
set "BASE_PORT=9000"
if exist "%CONFIG_FILE%" (
  echo Reading port from %CONFIG_FILE%...
  for /f "tokens=2 delims=: " %%a in ('findstr "port:" "%CONFIG_FILE%"') do (
    set "BASE_PORT=%%a"
  )
)
set /a "WEB_PORT=BASE_PORT + 2"
echo Base port: !BASE_PORT! - Web panel port: !WEB_PORT!

REM 0) Build the RELAY using root build.bat
if not exist "build.bat" (
  echo ERROR: Root build.bat not found.
  goto FAIL
)
echo Running root build.bat (relay)...
call build.bat
if errorlevel 1 (
  echo ERROR: build.bat failed.
  goto FAIL
)

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

REM Step 4: Update .env files with the correct web port
echo Updating .env files with port !WEB_PORT!...
for %%f in (.env.development .env.production) do (
  if exist "%%f" (
    set "FOUND_BASE_URL=0"
    set "TEMP_ENV=%%f.tmp"
    if exist "!TEMP_ENV!" del "!TEMP_ENV!"
    for /f "usebackq delims=" %%a in ("%%f") do (
      set "envline=%%a"
      set "outline=!envline!"
      echo !envline! | findstr /B /C:"REACT_APP_BASE_URL=" >nul
      if not errorlevel 1 (
        set "outline=REACT_APP_BASE_URL=http://localhost:!WEB_PORT!"
        set "FOUND_BASE_URL=1"
      )
      echo !outline!>> "!TEMP_ENV!"
    )
    if "!FOUND_BASE_URL!"=="0" (
      echo REACT_APP_BASE_URL=http://localhost:!WEB_PORT!>> "!TEMP_ENV!"
    )
    move /y "!TEMP_ENV!" "%%f" >nul
  )
)

REM Step 5: Install dependencies
echo Installing dependencies with yarn...
call yarn install || echo Warning: Yarn install may have failed.

REM Step 6: Run the panel's own build script
echo Running build.bat inside panel-source...
call build.bat
if errorlevel 1 (
    echo Error: Panel build.bat failed.
    popd
    goto end
)

REM Step 7: Return to root and copy build output
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
echo 1. Start your relay server: hornet-storage.exe
echo 2. Visit http://localhost:!WEB_PORT!
echo 3. The panel should load automatically

:end
pause
endlocal

:FAIL
exit /b 1
