#!/bin/bash

# CFD Platform Setup Script for macOS
# This script installs and configures the local development environment

set -e

echo "🚀 CFD/FEA Platform Setup"
echo "=========================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    echo -e "${RED}Error: This script is designed for macOS${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Running on macOS"

# Check if Homebrew is installed
if ! command -v brew &> /dev/null; then
    echo -e "${YELLOW}Installing Homebrew...${NC}"
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi
echo -e "${GREEN}✓${NC} Homebrew installed"

# Install Docker Desktop if not installed
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}Please install Docker Desktop from https://www.docker.com/products/docker-desktop${NC}"
    echo "Then run this script again"
    exit 1
fi
echo -e "${GREEN}✓${NC} Docker installed"

# Install kubectl
if ! command -v kubectl &> /dev/null; then
    echo -e "${YELLOW}Installing kubectl...${NC}"
    brew install kubectl
fi
echo -e "${GREEN}✓${NC} kubectl installed"

# Install kind (Kubernetes in Docker)
if ! command -v kind &> /dev/null; then
    echo -e "${YELLOW}Installing kind...${NC}"
    brew install kind
fi
echo -e "${GREEN}✓${NC} kind installed"

# Install Go
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Installing Go...${NC}"
    brew install go
fi
echo -e "${GREEN}✓${NC} Go installed ($(go version))"

# Create kind cluster
echo -e "${YELLOW}Creating Kubernetes cluster...${NC}"
if kind get clusters | grep -q "cfd-platform"; then
    echo "Cluster already exists, deleting..."
    kind delete cluster --name cfd-platform
fi

cat <<EOF | kind create cluster --name cfd-platform --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
EOF

echo -e "${GREEN}✓${NC} Kubernetes cluster created"

# Set kubectl context
kubectl cluster-info --context kind-cfd-platform
echo -e "${GREEN}✓${NC} kubectl context set"

# Create namespace and RBAC
echo -e "${YELLOW}Configuring Kubernetes resources...${NC}"
kubectl apply -f k8s/deployment.yaml

echo -e "${GREEN}✓${NC} Kubernetes resources created"

# Build backend
echo -e "${YELLOW}Building backend...${NC}"
cd backend
go mod download
go build -o cfd-platform-backend .
echo -e "${GREEN}✓${NC} Backend built"

# Create data directories
mkdir -p ../data/{inputs,results}

echo ""
echo -e "${GREEN}✅ Setup complete!${NC}"
echo ""
echo "Next steps:"
echo "1. Start the backend: cd backend && ./cfd-platform-backend"
echo "2. Open browser: http://localhost:8080"
echo ""
echo "To test with example data:"
echo "  - CFD: Use OpenFOAM case directory"
echo "  - FEA: Use CalculiX input file (.inp)"
echo ""
echo "Useful commands:"
echo "  - View cluster: kind get clusters"
echo "  - View pods: kubectl get pods -n cfd-platform"
echo "  - View jobs: kubectl get jobs -n cfd-platform"
echo "  - Delete cluster: kind delete cluster --name cfd-platform"
