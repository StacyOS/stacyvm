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
# Using find with -type d to ensure we only look at directories
for dir in $(find "$IMAGE_DIR" -maxdepth 1 -mindepth 1 -type d); do
    if [ ! -f "$dir/Dockerfile" ]; then
        continue
    fi

    template_name=$(basename "$dir")
    image_tag="stacyvm-$template_name"
    
    echo ""
    echo "📦 Processing: $template_name"
    echo "----------------------------------------"
    
    # 1. Build the Docker image
    echo "🔨 Building Docker image: $image_tag..."
    docker build -t "$image_tag" "$dir"
    
    # 2. Register/Update the template in StacyVM
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

    # Check if server is reachable for registration using /health
    # We'll try a simple GET and check for any response
    echo "🔍 Checking StacyVM server readiness at $STACYVM_SERVER..."
    if curl -s -f "$STACYVM_SERVER/api/v1/health" > /dev/null; then
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
        echo "⚠️ StacyVM server at $STACYVM_SERVER/api/v1/health is not reachable."
        echo "   Skipping registration for '$template_name'."
        echo "   (You can still spawn sandboxes using the image '$image_tag' manually)"
    fi

    # 3. Optional: Firecracker Rootfs Conversion
    if [[ "$1" == "--firecracker" ]]; then
        echo "🔥 Converting to Firecracker rootfs (Size: ${DISK_SIZE_MB}MB)..."
        ./stacyvm build-image "$image_tag" --disk-size "$DISK_SIZE_MB"
    fi
done

echo ""
echo "✨ Sync Complete."
