# scripts/windows-run.ps1
# Windows 下的简易运行脚本（兜底，无 Make 时使用）。

param(
    [string]$Action = "run"
)

switch ($Action) {
    "run"    { go run ./cmd/worktide }
    "build"  { go build -o bin/worktide.exe ./cmd/worktide }
    "test"   { go test ./... }
    "vet"    { go vet ./... }
    "fmt"    { gofmt -s -w . }
    default  { Write-Host "用法: .\scripts\windows-run.ps1 -Action run|build|test|vet|fmt"; exit 1 }
}
