@echo off
rem Ludotheque ST : ouverture automatique a la connexion Windows.
set "CIBLE=%~dp0Ludotheque.exe"
powershell -NoProfile -Command "$s=(New-Object -ComObject WScript.Shell).CreateShortcut([Environment]::GetFolderPath('Startup')+'\Ludotheque ST.lnk'); $s.TargetPath=$env:CIBLE; $s.WorkingDirectory='%~dp0'; $s.Save()"
echo La ludotheque s'ouvrira a chaque connexion.
pause
