#!/bin/bash
set -e

# Configuration
IMAGE_DIR="images"
STACYVM_SERVER="${STACYVM_SERVER:-http://localhost:7423}"
STACYVM_API_KEY="${STACYVM_API_KEY:-}"
DISK_SIZE_MB=3072  # 3GB (Next.js is heavy)

# Check for dependencies
if ! command -v docker &> /dev/null; then
    echo "Error: docker is required but not installed."
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed."
    exit 1
fi

echo "🚀 Starting StacyVM Template Sync (Auto-Build & Register)..."

# Find all directories in images/ that contain a Dockerfile
for dir in $(find "$IMAGE_DIR" -maxdepth 2 -name "Dockerfile" -exec dirname {} \;); do
    template_name=$(basename "$dir")
    image_tag="stacy-$template_name"
    
    echo ""
    echo "📦 Processing: $template_name"
    echo "----------------------------------------"
    
    # 1. Build the Docker image
    echo "🔨 Building Docker image: $image_tag..."
    # Use --pull to ensure we have the latest base image
    docker build -t "$image_tag" "$dir"
    
    # 2. Register/Update the template in StacyVM
    # Construction of JSON payload
    payload=$(jq -n \
        --arg name "$template_name" \
        --arg image "$image_tag" \
        --arg desc "Auto-synced from images/$template_name" \
        --argjson mem 2048 \
        --argjson cpu 2 \
        '{
            name: $name,
            image: $image,
            description: $desc,
            memory_mb: $mem,
            cpu_cores: $cpu,
            ttl_seconds: 3600
        }')

    # Check if server is reachable for registration
    status_code=$(curl -s -o /dev/null -w "%{http_code}" "$STACYVM_SERVER/api/v1/health")
    if [ "$status_code" == "200" ]; then
        echo "📝 Registering template '$template_name' with StacyVM..."
        
        response=$(curl -s -X POST "$STACYVM_SERVER/api/v1/templates" \
            -H "Content-Type: application/json" \
            -H "X-API-Key: $STACYVM_API_KEY" \
            -d "$payload")
        
        if echo "$response" | grep -q "already exists"; then
            echo "🔄 Template exists, updating..."
            curl -s -X PUT "$STACYVM_SERVER/api/v1/templates/$template_name" \
                -H "Content-Type: application/json" \
                -H "X-API-Key: $STACYVM_API_KEY" \
                -d "$payload" > /dev/null
            echo "✅ Update successful."
        elif echo "$response" | grep -q "\"name\""; then
            echo "✅ Registration successful."
        else
            echo "❌ Registration failed: $response"
        fi
    else
        echo "⚠️ StacyVM server at $STACYVM_SERVER is not reachable. Skipping registration."
    fi

    # 3. Optional: Firecracker Rootfs Conversion
    # We only do this if specifically requested via flag or if we detect firecracker is the target
    if [[ "$1" == "--firecracker" ]]; then
        echo "🔥 Converting to Firecracker rootfs (Size: ${DISK_SIZE_MB}MB)..."
        ./stacyvm build-image "$image_tag" --disk-size "$DISK_SIZE_MB"
    fi
done

echo ""
echo "✨ Sync Complete."
if [[ "$1" != "--firecracker" ]]; then
    echo "💡 Note: If you want to use Firecracker VMs instead of Docker, run: ./scripts/sync-templates.sh --firecracker"
fi
