@echo off
rem Inicia o servidor local carregando o .env.
rem Variaveis ja definidas no ambiente tem prioridade sobre o .env.
setlocal EnableExtensions DisableDelayedExpansion

pushd "%~dp0" || exit /b 1

where go >nul 2>&1
if errorlevel 1 (
    echo Erro: Go nao encontrado no PATH. 1>&2
    popd
    exit /b 1
)

if not exist ".env" (
    copy /y ".env.example" ".env" >nul
    echo Arquivo .env criado a partir de .env.example.
)

for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do (
    if not defined %%A set "%%A=%%B"
)

go run ./cmd/termofeed %*
set "EXIT_CODE=%ERRORLEVEL%"

popd
exit /b %EXIT_CODE%
