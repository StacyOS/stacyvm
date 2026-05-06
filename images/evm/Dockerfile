# Use Node.js 22 as the base for a modern Next.js environment
FROM oven/bun:latest

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# ── System packages + CLI tools (Common AI Sandbox utilities) ──
# This ensures the sandbox is capable of handling more than just web code
RUN apt-get update && apt-get install -y \
    # Core build tools and utilities
    curl wget git zip unzip jq vim nano htop \
    net-tools iputils-ping build-essential pkg-config \
    # Media & Document processing (Useful for AI agents)
    ffmpeg imagemagick \
    poppler-utils qpdf \
    # Python support (Often needed for auxiliary scripts)
    python3 python3-pip python3-venv \
    # Chromium dependencies (for Playwright/Puppeteer)
    libasound2 libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 \
    libxkbcommon0 libxcomposite1 libxdamage1 libxext6 libxfixes3 libxrandr2 \
    libgbm1 libpango-1.0-0 libcairo2 \
    && rm -rf /var/lib/apt/lists/*

# ── Install Foundry ──
RUN curl -L https://foundry.paradigm.xyz | bash
ENV PATH="/root/.foundry/bin:${PATH}"
RUN foundryup


# ── Set up the Workspace ──
WORKDIR /workspace

# ── Install Node.js Dependencies ──
# Copy frontend package files first to leverage Docker's layer caching
COPY frontend/package.json frontend/bun.lock ./frontend/
RUN cd frontend && bun install

# ── Copy Environment Files ──
# Explicitly copy .env files to ensure they are included early or if others are skipped
COPY contracts/.env* ./contracts/
COPY frontend/.env* ./frontend/

# ── Copy Template Code ──
# Copy the rest of the project into the image
COPY . .

# ── Environment Configuration ──
ENV NODE_ENV=development
ENV PORT=3000

# Expose the default Next.js port
EXPOSE 3000

# StacyVM Note: 
# 1. When running via Firecracker, stacyvm-agent is injected as /sbin/init.
# 2. When running via Docker, this CMD keeps the container alive for exec calls.
CMD ["sh", "-c", "sleep infinity"]
