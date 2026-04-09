# NogoChain Explorer Nginx Dockerfile
# Production-grade Nginx configuration with Let's Encrypt support

FROM nginx:1.25-alpine

# Install certbot for Let's Encrypt
RUN apk add --no-cache certbot openssl

# Create necessary directories
RUN mkdir -p /var/www/certbot \
    && mkdir -p /etc/letsencrypt \
    && mkdir -p /var/log/nginx \
    && mkdir -p /etc/nginx/ssl

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Copy custom configuration files if they exist
COPY conf.d/ /etc/nginx/conf.d/

# Generate self-signed certificate for IP-based access (fallback)
RUN openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/nginx/ssl/selfsigned.key \
    -out /etc/nginx/ssl/selfsigned.crt \
    -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"

# Expose ports
EXPOSE 80 443 8443

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost/health || exit 1

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
