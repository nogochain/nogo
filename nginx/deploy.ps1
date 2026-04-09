# NogoChain Nginx Deployment Script (PowerShell)
# Automates the setup and deployment of Nginx reverse proxy

param(
    [string]$DomainName = "explorer.nogochain.org",
    [string]$AdminEmail = "admin@nogochain.org",
    [string]$BrandPrefix = "nogo",
    [switch]$SkipSSLCert,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

# Colors
function Write-Success {
    param([string]$Message)
    Write-Host "[✓] $Message" -ForegroundColor Green
}

function Write-Status {
    param([string]$Message)
    Write-Host "[*] $Message" -ForegroundColor Yellow
}

function Write-Error-Custom {
    param([string]$Message)
    Write-Host "[✗] $Message" -ForegroundColor Red
}

# Script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "==================================" -ForegroundColor Green
Write-Host "NogoChain Nginx Deployment Script" -ForegroundColor Green
Write-Host "==================================" -ForegroundColor Green
Write-Host ""

# Check prerequisites
Write-Status "Checking prerequisites..."

try {
    $dockerVersion = docker --version
    Write-Success "Docker is installed: $dockerVersion"
} catch {
    Write-Error-Custom "Docker is not installed. Please install Docker Desktop first."
    exit 1
}

# Create directory structure
Write-Status "Creating directory structure..."

$directories = @(
    "$ScriptDir\certbot\conf",
    "$ScriptDir\certbot\www",
    "$ScriptDir\logs",
    "$ScriptDir\ssl"
)

foreach ($dir in $directories) {
    if (!(Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
}

Write-Success "Directory structure created"

# Setup environment file
Write-Status "Setting up environment file..."

$envExamplePath = "$ScriptDir\.env.example"
$envPath = "$ScriptDir\.env"

if (!(Test-Path $envPath)) {
    if (Test-Path $envExamplePath) {
        Copy-Item $envExamplePath $envPath
        Write-Success "Environment file created from example"
    } else {
        # Create default .env file
        $envContent = @"
# NogoChain Nginx Environment Variables
DOMAIN_NAME=$DomainName
ADMIN_EMAIL=$AdminEmail
BRAND_PREFIX=$BrandPrefix
"@
        $envContent | Out-File -FilePath $envPath -Encoding UTF8
        Write-Success "Environment file created"
    }
} else {
    Write-Success "Environment file already exists"
}

# Generate self-signed certificate
Write-Status "Generating self-signed certificate for IP access..."

$certPath = "$ScriptDir\ssl\selfsigned.crt"
$keyPath = "$ScriptDir\ssl\selfsigned.key"

if (!(Test-Path $certPath) -or !(Test-Path $keyPath)) {
    try {
        $opensslArgs = @(
            "req", "-x509", "-nodes", "-days", "365", "-newkey", "rsa:2048",
            "-keyout", $keyPath,
            "-out", $certPath,
            "-subj", "/C=US/ST=State/L=City/O=NogoChain/CN=localhost"
        )
        
        & openssl $opensslArgs 2>$null
        
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Self-signed certificate generated"
        } else {
            throw "OpenSSL command failed"
        }
    } catch {
        Write-Error-Custom "Failed to generate self-signed certificate"
        Write-Status "Continuing without self-signed certificate..."
    }
} else {
    Write-Success "Self-signed certificate already exists"
}

# Get SSL certificate using Certbot (optional)
if (!$SkipSSLCert) {
    Write-Status "Attempting to obtain SSL certificate from Let's Encrypt..."
    
    # Check DNS resolution
    try {
        $dnsResult = Resolve-DnsName -Name $DomainName -ErrorAction Stop
        Write-Success "Domain $DomainName resolves to: $($dnsResult.IP4Address)"
    } catch {
        Write-Error-Custom "Domain $DomainName does not resolve. Please configure DNS first."
        Write-Host ""
        Write-Host "DNS Configuration Required:" -ForegroundColor Yellow
        Write-Host "  Type: A"
        Write-Host "  Host: explorer"
        Write-Host "  Value: Your server IP address"
        Write-Host ""
        
        $continue = Read-Host "Continue without SSL certificate? (y/N)"
        if ($continue -ne "y" -and $continue -ne "Y") {
            exit 1
        }
    }
    
    # Try to get certificate
    if ($dnsResult) {
        try {
            $certbotArgs = @(
                "run", "--rm", "-it",
                "-v", "${ScriptDir}\certbot\conf:/etc/letsencrypt",
                "-v", "${ScriptDir}\certbot\www:/var/www/certbot",
                "certbot/certbot", "certonly",
                "--webroot",
                "--webroot-path=/var/www/certbot",
                "--email", $AdminEmail,
                "--agree-tos",
                "--no-eff-email",
                "-d", $DomainName,
                "--non-interactive"
            )
            
            & docker $certbotArgs
            
            if ($LASTEXITCODE -eq 0) {
                Write-Success "SSL certificate obtained successfully"
            } else {
                throw "Certbot failed"
            }
        } catch {
            Write-Error-Custom "Failed to obtain SSL certificate"
            Write-Status "You can try again later or continue with self-signed certificate"
        }
    }
}

# Start Nginx service
Write-Status "Starting Nginx service..."

Set-Location "$ScriptDir\.."

try {
    & docker-compose "-f" "nginx/docker-compose.nginx.yml" "up" "-d"
    
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Docker Compose started successfully"
    } else {
        throw "Docker Compose failed"
    }
} catch {
    Write-Error-Custom "Failed to start Nginx service"
    Write-Status "Check logs with: docker-compose -f nginx/docker-compose.nginx.yml logs"
    exit 1
}

# Wait for service to start
Write-Status "Waiting for service to start..."
Start-Sleep -Seconds 5

# Check if service is running
$containerName = "${BrandPrefix}-nginx"
$runningContainers = docker ps --format "{{.Names}}"

if ($runningContainers -like "*$containerName*") {
    Write-Success "Nginx service started successfully"
} else {
    Write-Error-Custom "Failed to start Nginx service"
    Write-Status "Check logs with: docker-compose -f nginx/docker-compose.nginx.yml logs -f"
    exit 1
}

# Verify deployment
Write-Status "Verifying deployment..."

Start-Sleep -Seconds 3

try {
    $response = Invoke-WebRequest -Uri "https://$DomainName" -UseBasicParsing -SkipCertificateCheck -TimeoutSec 10
    if ($response.StatusCode -in @(200, 301, 302)) {
        Write-Success "Nginx is responding (Status: $($response.StatusCode))"
    } else {
        Write-Error-Custom "Nginx responded with unexpected status: $($response.StatusCode)"
    }
} catch {
    Write-Error-Custom "Failed to connect to Nginx: $_"
}

# Check SSL certificate
if (Test-Path "$ScriptDir\certbot\conf\live\$DomainName\fullchain.pem") {
    Write-Success "SSL certificate is installed"
} else {
    Write-Status "SSL certificate not found (will be obtained on first access or use self-signed)"
}

# Print summary
Write-Host ""
Write-Host "==================================" -ForegroundColor Green
Write-Host "Deployment Summary" -ForegroundColor Green
Write-Host "==================================" -ForegroundColor Green
Write-Host ""
Write-Host "Domain: $DomainName"
Write-Host "Admin Email: $AdminEmail"
Write-Host "Container Name: $containerName"
Write-Host ""
Write-Host "Access URLs:" -ForegroundColor Cyan
Write-Host "  - HTTPS: https://$DomainName"
Write-Host "  - HTTP:  http://$DomainName (redirects to HTTPS)"
Write-Host "  - IP:    https://<your-server-ip>:8443"
Write-Host ""
Write-Host "Useful Commands:" -ForegroundColor Cyan
Write-Host "  - Status:   docker-compose -f nginx/docker-compose.nginx.yml ps"
Write-Host "  - Logs:     docker-compose -f nginx/docker-compose.nginx.yml logs -f"
Write-Host "  - Restart:  docker-compose -f nginx/docker-compose.nginx.yml restart"
Write-Host "  - Stop:     docker-compose -f nginx/docker-compose.nginx.yml down"
Write-Host "  - Reload:   docker exec $containerName nginx -s reload"
Write-Host ""
Write-Success "Deployment completed successfully!"
