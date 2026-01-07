@echo off
setlocal EnableExtensions EnableDelayedExpansion

REM Always operate from the script's folder (repo root)
pushd "%~dp0" >nul

REM --- Config ---
set "REPO_URL=https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git"
set "PANEL_DIR=panel-source"
set "BACKEND_EXE=hornet-storage.exe"
set "CONFIG_FILE=config.yaml"
set "NODE_OPTIONS=--openssl-legacy-provider --max-old-space-size=4096"
REM -------------

echo(
echo ================================
echo HORNETS-Relay-Panel Dev Runner
echo ================================
echo(

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

REM 1) Clone panel if missing (no pull/update if it already exists)
if not exist "%PANEL_DIR%" (
  where git >nul 2>nul || (echo ERROR: git not found in PATH.& goto FAIL)
  echo Cloning panel to %PANEL_DIR% ...
  git clone "%REPO_URL%" "%PANEL_DIR%" || (echo ERROR: clone failed.& goto FAIL)
)

REM Sanity check the panel project exists
if not exist "%PANEL_DIR%\package.json" (
  echo ERROR: %PANEL_DIR%\package.json not found.
  goto FAIL
)

REM Update .env.development with the correct web port
echo Updating .env.development with port !WEB_PORT!...
if exist "%PANEL_DIR%\.env.development" (
  set "FOUND_BASE_URL=0"
  set "TEMP_ENV=%PANEL_DIR%\.env.tmp"
  if exist "!TEMP_ENV!" del "!TEMP_ENV!"
  for /f "usebackq delims=" %%a in ("%PANEL_DIR%\.env.development") do (
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
  move /y "!TEMP_ENV!" "%PANEL_DIR%\.env.development" >nul
)

REM 2) Build the RELAY using root build.bat
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

REM 3) Run the backend exe from the root
if not exist "%BACKEND_EXE%" (
  echo ERROR: %BACKEND_EXE% not found in repo root after build.
  echo Tip: confirm build.bat outputs the exe to the root, or adjust BACKEND_EXE path.
  goto FAIL
)
echo Starting backend: %BACKEND_EXE%
start "" "%BACKEND_EXE%"

REM 4) Start the panel in dev mode (current window)
echo(
echo Starting panel dev server (dev mode)...
pushd "%PANEL_DIR%" >nul

REM Install deps (Yarn preferred, fallback to npm)
where yarn >nul 2>nul
if errorlevel 1 (
  call npm install
  if errorlevel 1 echo WARNING: npm install reported issues.
) else (
  call yarn install
  if errorlevel 1 echo WARNING: yarn install reported issues.
)

REM Create themes directory if it doesn't exist and build themes
echo Building themes for development...
if not exist "public\themes" mkdir "public\themes"
call node_modules\.bin\lessc --js --clean-css="--s1 --advanced" src/styles/themes/main.less public/themes/main.css
if errorlevel 1 (
  echo WARNING: Theme building failed. Styles may not load properly.
)

REM Prefer CRACO if present; else yarn start; else npm start
if exist "node_modules\.bin\craco" (
  call npx craco start
  set "RC=%ERRORLEVEL%"
) else (
  where yarn >nul 2>nul
  if errorlevel 1 (
    set "NODE_ENV=development"
    call npm run start
    set "RC=%ERRORLEVEL%"
  ) else (
    set "NODE_ENV=development"
    call yarn start
    set "RC=%ERRORLEVEL%"
  )
)

popd >nul
popd >nul
exit /b %RC%

:FAIL
popd >nul
exit /b 1
