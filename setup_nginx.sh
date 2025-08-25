#!/bin/bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
        OS_NAME=$PRETTY_NAME
    else
        log_error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi
}

detect_architecture() {
    ARCH=$(uname -m)
    case $ARCH in
        aarch64|arm64)
            log_info "Detected ARM64 architecture (Raspberry Pi compatible)"
            ;;
        x86_64)
            log_info "Detected x86_64 architecture"
            ;;
        armv7l)
            log_info "Detected ARMv7 architecture (32-bit Raspberry Pi)"
            ;;
        *)
            log_warning "Unknown architecture: $ARCH"
            ;;
    esac
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root. Please use sudo."
        exit 1
    fi
}

check_nginx_installed() {
    if command -v nginx &> /dev/null; then
        NGINX_VERSION=$(nginx -v 2>&1 | cut -d' ' -f3 | cut -d'/' -f2)
        log_info "Nginx is already installed (version: $NGINX_VERSION)"
        return 0
    else
        log_info "Nginx is not installed"
        return 1
    fi
}

install_nginx_debian() {
    log_info "Installing nginx on Debian/Ubuntu/Raspberry Pi OS..."
    
    apt-get update
    apt-get install -y nginx
    
    systemctl enable nginx
    
    log_success "Nginx installed successfully"
}

install_nginx_redhat() {
    log_info "Installing nginx on RHEL/CentOS/Fedora..."
    
    if command -v dnf &> /dev/null; then
        dnf install -y nginx
    else
        yum install -y epel-release
        yum install -y nginx
    fi
    
    systemctl enable nginx
    
    log_success "Nginx installed successfully"
}

install_nginx_arch() {
    log_info "Installing nginx on Arch Linux..."
    
    pacman -Sy --noconfirm nginx
    
    systemctl enable nginx
    
    log_success "Nginx installed successfully"
}

install_nginx() {
    case $OS in
        debian|ubuntu|raspbian)
            install_nginx_debian
            ;;
        rhel|centos|fedora|rocky|almalinux)
            install_nginx_redhat
            ;;
        arch|manjaro)
            install_nginx_arch
            ;;
        *)
            log_error "Unsupported OS: $OS"
            log_info "Please install nginx manually and run this script again."
            exit 1
            ;;
    esac
}

backup_existing_config() {
    NGINX_CONFIG_DIR="/etc/nginx"
    BACKUP_DIR="/etc/nginx/backups"
    TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
    
    if [ ! -d "$BACKUP_DIR" ]; then
        mkdir -p "$BACKUP_DIR"
    fi
    
    if [ -f "$NGINX_CONFIG_DIR/sites-available/hornets-relay" ]; then
        log_info "Backing up existing configuration..."
        cp "$NGINX_CONFIG_DIR/sites-available/hornets-relay" "$BACKUP_DIR/hornets-relay.$TIMESTAMP"
    fi
    
    if [ -f "$NGINX_CONFIG_DIR/nginx.conf" ]; then
        cp "$NGINX_CONFIG_DIR/nginx.conf" "$BACKUP_DIR/nginx.conf.$TIMESTAMP"
    fi
}

write_nginx_config() {
    local CONFIG_FILE="$1"
    
    cat > "$CONFIG_FILE" << 'NGINX_CONFIG'
# Define upstream servers for each service (using explicit IPv4 addresses)
upstream transcribe_api {
    server 127.0.0.1:8000;
}

upstream relay_service {
    server 127.0.0.1:9001;
}

upstream panel_service {
    server 127.0.0.1:9002;
}

upstream wallet_service {
    server 127.0.0.1:9003;
}

# WebSocket connection upgrade mapping
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

# Main server block listening on HTTP
server {
    listen 80; # Nginx listens on port 80 locally
    server_name _; # Accept all hostnames (localhost, ngrok, custom domains, etc.)

    # Basic Security Headers
    add_header X-Frame-Options "SAMEORIGIN";
    add_header X-Content-Type-Options "nosniff";
    add_header X-XSS-Protection "1; mode=block";
    server_tokens off;

    # Increase buffer sizes for large files
    client_max_body_size 100M;

    # Forward client IP and protocol
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Host $host;

    # Health check endpoint - exact match first
    location = /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }

    # Relay WebSocket service - handle both /relay and /relay/
    location ~ ^/relay/?$ {
        # Strip the /relay prefix (with or without trailing slash) when forwarding to the service
        rewrite ^/relay/?$ / break;
        
        proxy_pass http://relay_service;
        
        # WebSocket-specific headers
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        
        # Extended timeouts for WebSocket connections
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
        proxy_connect_timeout 60s;
        
        # Additional headers for tunnel compatibility
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # Transcribe service
    location /transcribe/ {
        rewrite ^/transcribe/(.*)$ /$1 break;
        proxy_pass http://transcribe_api;
    }

    # Wallet service
    location /wallet/ {
        rewrite ^/wallet/(.*)$ /$1 break;
        proxy_pass http://wallet_service;
    }

    # Blossom file storage routes
    location /blossom/ {
        proxy_pass http://panel_service;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Disable buffering for file uploads/downloads
        proxy_buffering off;
        proxy_request_buffering off;
        
        # Set appropriate headers
        proxy_set_header Accept-Encoding "";
        
        # Larger timeouts for file operations
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
        proxy_connect_timeout 60s;
    }

    # Default location - Panel service (frontend + API) - MUST BE LAST
    location / {
        # Add CORS headers for the panel service
        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, PUT, DELETE, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'Origin, Content-Type, Accept, Authorization' always;

        # Handle preflight OPTIONS requests
        if ($request_method = 'OPTIONS') {
            add_header 'Access-Control-Allow-Origin' '*';
            add_header 'Access-Control-Allow-Methods' 'GET, POST, PUT, DELETE, OPTIONS';
            add_header 'Access-Control-Allow-Headers' 'Origin, Content-Type, Accept, Authorization';
            add_header 'Content-Length' 0;
            return 204;
        }

        proxy_pass http://panel_service;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Handle WebSocket if needed
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
NGINX_CONFIG
}

setup_nginx_config() {
    log_info "Setting up nginx configuration..."
    
    NGINX_CONFIG_DIR="/etc/nginx"
    CONFIG_FILE="$NGINX_CONFIG_DIR/sites-available/hornets-relay"
    ENABLED_LINK="$NGINX_CONFIG_DIR/sites-enabled/hornets-relay"
    
    if [ ! -d "$NGINX_CONFIG_DIR/sites-available" ]; then
        mkdir -p "$NGINX_CONFIG_DIR/sites-available"
    fi
    
    if [ ! -d "$NGINX_CONFIG_DIR/sites-enabled" ]; then
        mkdir -p "$NGINX_CONFIG_DIR/sites-enabled"
    fi
    
    log_info "Writing nginx configuration..."
    write_nginx_config "$CONFIG_FILE"
    
    if [ -L "$ENABLED_LINK" ]; then
        rm "$ENABLED_LINK"
    fi
    ln -s "$CONFIG_FILE" "$ENABLED_LINK"
    
    if [ -L "$NGINX_CONFIG_DIR/sites-enabled/default" ]; then
        log_info "Disabling default site..."
        rm "$NGINX_CONFIG_DIR/sites-enabled/default"
    fi
    
    log_success "Nginx configuration deployed"
}

test_nginx_config() {
    log_info "Testing nginx configuration..."
    
    if nginx -t 2>&1; then
        log_success "Nginx configuration is valid"
        return 0
    else
        log_error "Nginx configuration test failed"
        return 1
    fi
}

check_services() {
    log_info "Checking if backend services are running..."
    
    SERVICES=(
        "127.0.0.1:8000:Transcribe API"
        "127.0.0.1:9001:Relay Service"
        "127.0.0.1:9002:Panel Service"
        "127.0.0.1:9003:Wallet Service"
    )
    
    for service in "${SERVICES[@]}"; do
        IFS=':' read -r host port name <<< "$service"
        if nc -z "$host" "$port" 2>/dev/null; then
            log_success "$name is running on $host:$port"
        else
            log_warning "$name is NOT running on $host:$port"
        fi
    done
}

start_nginx() {
    log_info "Starting nginx service..."
    
    systemctl stop nginx 2>/dev/null || true
    
    systemctl start nginx
    
    if systemctl is-active --quiet nginx; then
        log_success "Nginx started successfully"
    else
        log_error "Failed to start nginx"
        systemctl status nginx
        exit 1
    fi
}

test_nginx_endpoints() {
    log_info "Testing nginx endpoints..."
    
    sleep 2
    
    if curl -s -o /dev/null -w "%{http_code}" http://localhost/health | grep -q "200"; then
        log_success "Health endpoint is responding"
    else
        log_warning "Health endpoint is not responding"
    fi
    
    NGINX_STATUS=$(systemctl is-active nginx)
    log_info "Nginx service status: $NGINX_STATUS"
    
    NGINX_PROCESS=$(pgrep -c nginx || echo "0")
    log_info "Nginx processes running: $NGINX_PROCESS"
    
    netstat -tlpn 2>/dev/null | grep :80 || ss -tlpn | grep :80
}

show_nginx_logs() {
    log_info "Recent nginx error logs:"
    tail -n 10 /var/log/nginx/error.log 2>/dev/null || echo "No error logs found"
}

setup_firewall() {
    log_info "Configuring firewall rules..."
    
    if command -v ufw &> /dev/null; then
        ufw allow 80/tcp
        ufw allow 443/tcp
        log_success "UFW firewall rules added"
    elif command -v firewall-cmd &> /dev/null; then
        firewall-cmd --permanent --add-service=http
        firewall-cmd --permanent --add-service=https
        firewall-cmd --reload
        log_success "Firewalld rules added"
    elif command -v iptables &> /dev/null; then
        iptables -A INPUT -p tcp --dport 80 -j ACCEPT
        iptables -A INPUT -p tcp --dport 443 -j ACCEPT
        log_success "Iptables rules added"
    else
        log_warning "No firewall detected, skipping firewall configuration"
    fi
}

print_summary() {
    echo ""
    echo "========================================="
    echo "       NGINX SETUP COMPLETE"
    echo "========================================="
    echo ""
    log_info "Configuration file: /etc/nginx/sites-available/hornets-relay"
    log_info "Nginx is listening on port 80"
    echo ""
    echo "Available endpoints:"
    echo "  - http://localhost/           (Panel Service)"
    echo "  - http://localhost/relay      (Relay WebSocket)"
    echo "  - http://localhost/transcribe (Transcribe API)"
    echo "  - http://localhost/wallet     (Wallet Service)"
    echo "  - http://localhost/blossom    (File Storage)"
    echo "  - http://localhost/health     (Health Check)"
    echo ""
    echo "Useful commands:"
    echo "  - sudo systemctl status nginx    (Check nginx status)"
    echo "  - sudo systemctl restart nginx   (Restart nginx)"
    echo "  - sudo nginx -t                  (Test configuration)"
    echo "  - sudo tail -f /var/log/nginx/error.log  (View error logs)"
    echo "  - sudo tail -f /var/log/nginx/access.log (View access logs)"
    echo ""
    
    if [ "$ARCH" == "aarch64" ] || [ "$ARCH" == "arm64" ] || [ "$ARCH" == "armv7l" ]; then
        echo "Raspberry Pi specific tips:"
        echo "  - Monitor CPU temp: vcgencmd measure_temp"
        echo "  - Check throttling: vcgencmd get_throttled"
        echo ""
    fi
}

main() {
    echo "========================================="
    echo "    HORNETS RELAY NGINX SETUP SCRIPT"
    echo "========================================="
    echo ""
    
    check_root
    
    detect_os
    log_info "Detected OS: $OS_NAME"
    
    detect_architecture
    
    if ! check_nginx_installed; then
        log_info "Installing nginx..."
        install_nginx
    fi
    
    backup_existing_config
    
    setup_nginx_config
    
    if ! test_nginx_config; then
        log_error "Configuration test failed. Rolling back..."
        if [ -f "$BACKUP_DIR/hornets-relay.$TIMESTAMP" ]; then
            cp "$BACKUP_DIR/hornets-relay.$TIMESTAMP" "$CONFIG_FILE"
        fi
        exit 1
    fi
    
    start_nginx
    
    setup_firewall
    
    check_services
    
    test_nginx_endpoints
    
    print_summary
    
    log_success "Nginx setup completed successfully!"
}

main "$@"