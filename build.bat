@echo off
rem Compile le lanceur pour Windows et Linux (Go 1.24+, sans CGO).
rem Fermer la ludothèque avant : Ludotheque.exe est verrouillé tant qu'elle tourne.
setlocal
cd /d "%~dp0sources\lanceur"
set CGO_ENABLED=0
set GOFLAGS=-mod=vendor
set GOOS=windows
go build -ldflags "-s -w -H windowsgui" -o ..\..\Ludotheque.exe . || goto erreur
set GOOS=linux
go build -ldflags "-s -w" -o ..\..\ludotheque . || goto erreur
echo Compilation OK : Ludotheque.exe et ludotheque
exit /b 0
:erreur
echo Echec de la compilation.
exit /b 1
