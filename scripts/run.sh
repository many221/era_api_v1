#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create necessary directories and files
create_project_structure() {
    # Create directories
    mkdir -p internal/{templates,handlers,parser,formatter,models}
    
    # Create default template if it doesn't exist
    if [ ! -f internal/templates/default.html ]; then
        cat > internal/templates/default.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>ERA API - Election Results</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .results { border: 1px solid #ddd; padding: 15px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Title}}</h1>
        <div class="results">
            {{.Content}}
        </div>
    </div>
</body>
</html>
EOF
        echo -e "${GREEN}Created default template${NC}"
    fi
}

echo -e "${GREEN}Setting up project structure...${NC}"
create_project_structure

echo -e "${GREEN}Starting ERA API server without CGO...${NC}"

# Disable CGO to bypass Xcode requirement
export CGO_ENABLED=0
export PORT=${PORT:-8080}
export GODEBUG=netdns=go # Force pure Go DNS resolution

# Run the server with pure Go
go run -tags netgo cmd/server/main.go