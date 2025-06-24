#!/bin/bash

# Script to switch between full frontend and minimal API-only mode

case "$1" in
    minimal)
        echo "Switching to minimal frontend (API-only mode)..."
        if [ -d "frontend" ] && [ ! -d "frontend-full" ]; then
            mv frontend frontend-full
            echo "Backed up full frontend to frontend-full/"
        fi
        cp -r frontend-minimal frontend
        echo "Switched to minimal frontend. You can now run: go run main.go"
        ;;
    full)
        echo "Switching to full frontend..."
        if [ -d "frontend-full" ]; then
            rm -rf frontend
            mv frontend-full frontend
            echo "Restored full frontend"
        else
            echo "No full frontend backup found at frontend-full/"
            exit 1
        fi
        ;;
    *)
        echo "Usage: $0 {minimal|full}"
        echo "  minimal - Switch to API-only mode with minimal frontend"
        echo "  full    - Restore full frontend (if backed up)"
        exit 1
        ;;
esac