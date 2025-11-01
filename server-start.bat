@echo off
REM Ensure using Go environment variables
cd /d %~dp0

chcp 65001 >nul

REM Set environment variable for this session
set JWT_SECRET="測試用環境_金鑰"

REM Execute main.go
go run main.go

pause
