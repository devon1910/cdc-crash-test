$ErrorActionPreference = 'Stop'
$env:GOCACHE = Join-Path $PSScriptRoot '.gocache'

Write-Warning 'This comparison permanently deletes this Compose project’s PostgreSQL and Debezium volumes before each scenario. Result files are kept.'
$confirmation = Read-Host 'Type YES to continue'
if ($confirmation -cne 'YES') { throw 'Comparison cancelled; no volumes were removed.' }

function Reset-Lab {
    Write-Host 'Removing Compose containers and volumes for a clean experiment baseline...'
    docker compose down --volumes --remove-orphans
    if ($LASTEXITCODE -ne 0) { throw 'docker compose down failed' }
}

function Start-Lab {
    docker compose up --build -d
    if ($LASTEXITCODE -ne 0) { throw 'docker compose up failed' }
    docker compose ps
}

try {
    foreach ($scenario in @('e1', 'e2-timer', 'e2')) {
        Reset-Lab
        Start-Lab
        Write-Host "Starting independent M4 scenario: $scenario"
        go run ./cmd/m4 "-scenario=$scenario"
        if ($LASTEXITCODE -ne 0) { throw "M4 scenario $scenario failed" }
    }
    Write-Host 'All M4 scenarios completed. Each began with fresh PostgreSQL and Debezium volumes.'
}
finally {
    Write-Host 'Stopping comparison containers while preserving the final run volumes.'
    docker compose down
}
