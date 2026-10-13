@echo off
rem Installe les images de disquettes et les TOS depuis la collection TOSEC Atari ST.
rem Glissez le dossier de la collection sur ce fichier (ou lancez-le et tapez le chemin).
cd /d "%~dp0"
set "PY="
where py >nul 2>nul && set "PY=py -3"
if not defined PY where python >nul 2>nul && set "PY=python"
if not defined PY (
  echo Python 3 est necessaire : winget install Python.Python.3.12
  pause
  exit /b 1
)
set "DOSSIER=%~1"
if not defined DOSSIER set /p "DOSSIER=Dossier de la collection TOSEC : "
%PY% outils\installer_images.py "%DOSSIER%"
pause
