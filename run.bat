for /f "tokens=1-2 delims=:" %%a in ('ipconfig^|find "IPv4"') do set ip=%%b
set ip=%ip:~1%
echo %ip%
start cmd /k go run front-end.go %ip%:3001
timeout /t 1
start cmd /k go run replica.go %ip%:3002 %ip%:3001 -1
timeout /t 1
start cmd /k go run replica.go %ip%:3003 %ip%:3001 