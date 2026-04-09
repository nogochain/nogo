#!/bin/bash

# NogoChain Nginx Deployment Script
# Automates the setup and deployment of Nginx reverse proxy

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
BRAND_PREFIX="${BRAND_PREFIX:-nogo}"
DOMAIN_NAME="${DOMAIN_NAME:-explorer.nogochain.org}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@nogochain.org}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}NogoChain Nginx Deployment Script${NC}"
echo -e "${GREEN}==================================${NC}"
echo ""

# Function to print status
print_status() {
    echo -e "${YELLOW}[*]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    
    print_success "Prerequisites check passed"
}

# Create directory structure
create_directories() {
    print_status "Creating directory structure..."
    
    mkdir -p "$SCRIPT_DIR/certbot/conf"
    mkdir -p "$SCRIPT_DIR/certbot/www"
    mkdir -p "$SCRIPT_DIR/logs"
    mkdir -p "$SCRIPT_DIR/ssl"
    
    print_success "Directory structure created"
}

# Generate self-signed certificate for IP access
generate_self_signed_cert() {
    print_status "Generating self-signed certificate for IP access..."
    
    if [ ! -f "$SCRIPT_DIR/ssl/selfsigned.crt" ] || [ ! -f "$SCRIPT_DIR/ssl/selfsigned.key" ]; then
        openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout "$SCRIPT_DIR/ssl/selfsigned.key" \
            -out "$SCRIPT_DIR/ssl/selfsigned.crt" \
            -subj "/C=US/ST=State/L=City/O=NogoChain/CN=localhost" \
            2>/dev/null
        
        print_success "Self-signed certificate generated"
    else
        print_success "Self-signed certificate already exists"
    fi
}

# Setup environment file
setup_env() {
    print_status "Setting up environment file..."
    
    if [ ! -f "$SCRIPT_DIR/.env" ]; then
        cp "$SCRIPT_DIR/.env.example" "$SCRIPT_DIR/.env"
        print_success "Environment file created from example"
    else
        print_success "Environment file already exists"
    fi
}

# Get SSL certificate using Certbot
get_ssl_certificate() {
    print_status "Attempting to obtain SSL certificate from Let's Encrypt..."
    
    # Check if domain resolves
    if ! host "$DOMAIN_NAME" &> /dev/null; then
        print_error "Domain $DOMAIN_NAME does not resolve. Please configure DNS first."
        echo ""
        echo "DNS Configuration Required:"
        echo "  Type: A"
        echo "  Host: explorer"
        echo "  Value: Your server IP address"
        echo ""
        read -p "Do you want to continue without SSL certificate? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
        return 1
    fi
    
    # Try to get certificate
    docker run --rm -it \
        -v "$SCRIPT_DIR/certbot/conf:/etc/letsencrypt" \
        -v "$SCRIPT_DIR/certbot/www:/var/www/certbot" \
        certbot/certbot certonly \
        --webroot \
        --webroot-path=/var/www/certbot \
        --email "$ADMIN_EMAIL" \
        --agree-tos \
        --no-eff-email \
        -d "$DOMAIN_NAME" \
        --non-interactive || {
            print_error "Failed to obtain SSL certificate"
            echo "You can try again later or use the self-signed certificate for testing"
            return 1
        }
    
    print_success "SSL certificate obtained successfully"
    return 0
}

# Start Nginx service
start_nginx() {
    print_status "Starting Nginx service..."
    
    cd "$SCRIPT_DIR/.."
    
    docker-compose -f nginx/docker-compose.nginx.yml up -d
    
    # Wait for service to start
    sleep 5
    
    # Check if service is running
    if docker ps | grep -q "${BRAND_PREFIX}-nginx"; then
        print_success "Nginx service started successfully"
    else
        print_error "Failed to start Nginx service"
        print_status "Check logs with: docker-compose -f nginx/docker-compose.nginx.yml logs"
        exit 1
    fi
}

# Verify deployment
verify_deployment() {
    print_status "Verifying deployment..."
    
    # Wait a bit for services to stabilize
    sleep 3
    
    # Check if Nginx is responding
    if curl -k -s -o /dev/null -w "%{http_code}" "https://$DOMAIN_NAME" | grep -q "200\|301\|302"; then
        print_success "Nginx is responding"
    else
        print_error "Nginx is not responding correctly"
    fi
    
    # Check SSL certificate
    if [ -f "$SCRIPT_DIR/certbot/conf/live/$DOMAIN_NAME/fullchain.pem" ]; then
        print_success "SSL certificate is installed"
    else
        print_status "SSL certificate not found (using self-signed or will be obtained on first access)"
    fi
}

# Print deployment summary
print_summary() {
    echo ""
    echo -e "${GREEN}==================================${NC}"
    echo -e "${GREEN}Deployment Summary${NC}"
    echo -e "${GREEN}==================================${NC}"
    echo ""
    echo "Domain: $DOMAIN_NAME"
    echo "Admin Email: $ADMIN_EMAIL"
    echo "Container Name: ${BRAND_PREFIX}-nginx"
    echo ""
    echo "Access URLs:"
    echo "  - HTTPS: https://$DOMAIN_NAME"
    echo "  - HTTP:  http://$DOMAIN_NAME (redirects to HTTPS)"
    echo "  - IP:    https://<your-server-ip>:8443"
    echo ""
    echo "Useful Commands:"
    echo "  - Status:   docker-compose -f nginx/docker-compose.nginx.yml ps"
    echo "  - Logs:     docker-compose -f nginx/docker-compose.nginx.yml logs -f"
    echo "  - Restart:  docker-compose -f nginx/docker-compose.nginx.yml restart"
    echo "  - Stop:     docker-compose -f nginx/docker-compose.nginx.yml down"
    echo "  - Reload:   docker exec ${BRAND_PREFIX}-nginx nginx -s reload"
    echo ""
    echo -e "${GREEN}Deployment completed successfully!${NC}"
}

# Main deployment function
main() {
    check_prerequisites
    create_directories
    setup_env
    generate_self_signed_cert
    
    # Try to get SSL certificate (optional)
    if get_ssl_certificate; then
        echo ""
    else
        echo ""
        print_status "Continuing without SSL certificate..."
    fi
    
    start_nginx
    verify_deployment
    print_summary
}

# Run main function
main "$@"
