@echo off
rem Ludotheque ST : retire l'ouverture automatique.
powershell -NoProfile -Command "Remove-Item -ErrorAction SilentlyContinue ([Environment]::GetFolderPath('Startup')+'\Ludotheque ST.lnk')"
echo Demarrage automatique retire.
pause
