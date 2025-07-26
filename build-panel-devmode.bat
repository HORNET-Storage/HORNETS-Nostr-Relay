@echo off
setlocal enabledelayedexpansion

echo.
echo ================================
echo HORNETS-Relay-Panel Development Build
echo ================================
echo.

REM Check if local panel-source exists
if not exist panel-source (
    echo Local panel-source directory not found!
    echo Creating panel-source and cloning from GitHub...
    echo.
    
    REM Clone the panel repository
    git clone https://github.com/HORNET-Storage/HORNETS-Relay-Panel.git panel-source
    if errorlevel 1 (
        echo ERROR: Failed to clone panel repository!
        echo Please check your internet connection and git installation.
        goto end
    )
    
    echo Successfully cloned panel repository!
    echo.
) else (
    echo Using existing local panel source...
    echo.
)

REM Navigate into panel-source
pushd panel-source

REM Check if it's a valid panel project
if not exist package.json (
    echo ERROR: panel-source doesn't appear to be a valid panel project - missing package.json!
    popd
    goto end
)

REM Install/update dependencies
echo Installing dependencies with yarn...
call yarn install
if errorlevel 1 (
    echo Warning: Yarn install may have encountered issues.
)

REM Build the project
echo Building panel...
REM Clear any existing build first
rmdir /S /Q build 2>nul

REM Check if build.bat exists and use it, otherwise use yarn build
if exist build.bat (
    echo Running build.bat...
    call build.bat
    if errorlevel 1 (
        echo Error: Panel build.bat failed.
        popd
        goto end
    )
) else (
    echo Running yarn build...
    set GENERATE_SOURCEMAP=false
    set NODE_ENV=production
    call yarn build
    if errorlevel 1 (
        echo Error: Yarn build failed.
        popd
        goto end
    )
)

REM Return to root and copy build output
popd
echo Copying build files to web directory...
REM Create web directory if it doesn't exist
if not exist web mkdir web
REM Clear any existing files
del /Q web\* 2>nul
for /d %%p in (web\*) do rmdir /S /Q "%%p" 2>nul
REM Copy built files
xcopy /E /I /Y panel-source\build\* web\

REM Final message
echo.
echo ================================
echo Build Complete!
echo ================================
echo Panel built and deployed successfully!
echo The panel is now available at your relay's root URL
echo.
echo Development workflow:
echo 1. Make changes in panel-source
echo 2. Run build-panel-devmode.bat to rebuild
echo 3. Refresh your browser to see changes
echo.
echo To test the panel:
echo 1. Start your relay server: go run services\server\port\main.go
echo 2. Visit http://localhost:9002 (or your configured port)
echo 3. The panel should load automatically
echo.

:end
pause
endlocal
