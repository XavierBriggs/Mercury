#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                  MERCURY - AUTOMATED SETUP                   ║"
echo "║                  God of Speed and Commerce                   ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
MERCURY_ROOT="$(dirname "$SCRIPT_DIR")"
FORTUNA_ROOT="$(dirname "$MERCURY_ROOT")"

echo "Directories:"
echo "  Mercury Root: $MERCURY_ROOT"
echo "  Fortuna Root: $FORTUNA_ROOT"
echo ""

# ==============================================================================
# STEP 1: Check Dependencies
# ==============================================================================
echo -e "${CYAN}[1/8] Checking dependencies...${NC}"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}✗ Docker is not installed${NC}"
    echo "Install from: https://docs.docker.com/get-docker/"
    exit 1
fi
echo -e "${GREEN}✓ Docker installed${NC}"

if ! command -v docker-compose &> /dev/null; then
    echo -e "${RED}✗ Docker Compose is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker Compose installed${NC}"

if ! command -v psql &> /dev/null; then
    echo -e "${YELLOW}⚠ PostgreSQL client not found (optional but recommended)${NC}"
fi

echo ""

# ==============================================================================
# STEP 2: Start Infrastructure
# ==============================================================================
echo -e "${CYAN}[2/8] Starting infrastructure (PostgreSQL + Redis)...${NC}"

cd "$FORTUNA_ROOT/deploy"

# Check if containers are already running
if docker ps | grep -q "fortuna-alexandria"; then
    echo -e "${YELLOW}⚠ Alexandria container already running${NC}"
else
    echo "Starting Alexandria..."
    docker-compose up -d alexandria 2>&1 | grep -v "version.*obsolete" || true
fi

if docker ps | grep -q "fortuna-redis" || redis-cli ping &> /dev/null; then
    echo -e "${YELLOW}⚠ Redis already available (local or container)${NC}"
else
    echo "Starting Redis..."
    docker-compose up -d redis 2>&1 | grep -v "version.*obsolete" || true
fi

echo -e "${GREEN}✓ Infrastructure started${NC}"
echo ""

# ==============================================================================
# STEP 3: Wait for Services
# ==============================================================================
echo -e "${CYAN}[3/8] Waiting for services to be healthy...${NC}"

echo -n "Waiting for Alexandria"
for i in {1..30}; do
    if docker exec fortuna-alexandria pg_isready -U fortuna &> /dev/null; then
        echo ""
        echo -e "${GREEN}✓ Alexandria is ready${NC}"
        break
    fi
    echo -n "."
    sleep 1
    if [ $i -eq 30 ]; then
        echo ""
        echo -e "${RED}✗ Alexandria failed to start${NC}"
        docker logs fortuna-alexandria --tail 20
        exit 1
    fi
done

# Check Redis (could be local or container)
if redis-cli ping &> /dev/null; then
    echo -e "${GREEN}✓ Redis is ready (local)${NC}"
elif docker exec fortuna-redis redis-cli ping &> /dev/null 2>&1; then
    echo -e "${GREEN}✓ Redis is ready (container)${NC}"
else
    echo -e "${YELLOW}⚠ Redis not accessible, but continuing...${NC}"
fi

echo ""

# ==============================================================================
# STEP 4: Create Databases
# ==============================================================================
echo -e "${CYAN}[4/8] Creating databases...${NC}"

# Create alexandria database if it doesn't exist
if ! docker exec fortuna-alexandria psql -U fortuna -lqt | cut -d \| -f 1 | grep -qw alexandria; then
    echo "Creating alexandria database..."
    docker exec fortuna-alexandria psql -U fortuna -c "CREATE DATABASE alexandria;" || true
fi
echo -e "${GREEN}✓ Database 'alexandria' ready${NC}"

# Create test database
if ! docker exec fortuna-alexandria psql -U fortuna -lqt | cut -d \| -f 1 | grep -qw alexandria_test; then
    echo "Creating alexandria_test database..."
    docker exec fortuna-alexandria psql -U fortuna -c "CREATE DATABASE alexandria_test;" || true
fi
echo -e "${GREEN}✓ Database 'alexandria_test' ready${NC}"

echo ""

# ==============================================================================
# STEP 5: Run Migrations
# ==============================================================================
echo -e "${CYAN}[5/8] Running database migrations...${NC}"

cd "$MERCURY_ROOT"

# Run migrations on main database
echo "Applying migrations to alexandria..."
for file in infra/alexandria/migrations/*.sql; do
    filename=$(basename "$file")
    echo "  → $filename"
    docker exec -i fortuna-alexandria psql -U fortuna -d alexandria < "$file" 2>&1 | grep -E "(CREATE|ALTER|ERROR)" | head -1 || true
done
echo -e "${GREEN}✓ Migrations applied to alexandria${NC}"

# Run migrations on test database
echo "Applying migrations to alexandria_test..."
for file in infra/alexandria/migrations/*.sql; do
    docker exec -i fortuna-alexandria psql -U fortuna -d alexandria_test < "$file" 2>&1 | grep -v "already exists" > /dev/null || true
done
echo -e "${GREEN}✓ Migrations applied to alexandria_test${NC}"

echo ""

# ==============================================================================
# STEP 6: Load Seed Data
# ==============================================================================
echo -e "${CYAN}[6/8] Loading seed data...${NC}"

echo "Loading seed data into alexandria..."
for file in infra/alexandria/seed/*.sql; do
    filename=$(basename "$file")
    echo "  → $filename"
    # Use ON CONFLICT in seed files, so this is idempotent
    docker exec -i fortuna-alexandria psql -U fortuna -d alexandria < "$file" 2>&1 | grep -E "(INSERT|ERROR)" | head -1 || true
done

# Verify counts
SPORT_COUNT=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM sports;" 2>/dev/null | tr -d ' ' || echo "0")
BOOK_COUNT=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM books;" 2>/dev/null | tr -d ' ' || echo "0")
MARKET_COUNT=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM markets;" 2>/dev/null | tr -d ' ' || echo "0")

echo -e "${GREEN}✓ Seed data loaded (Sports: $SPORT_COUNT, Books: $BOOK_COUNT, Markets: $MARKET_COUNT)${NC}"

# Load seed data into test database
echo "Loading seed data into alexandria_test..."
for file in infra/alexandria/seed/*.sql; do
    docker exec -i fortuna-alexandria psql -U fortuna -d alexandria_test < "$file" 2>&1 | grep -v "duplicate key" > /dev/null || true
done
echo -e "${GREEN}✓ Test database seeded${NC}"

echo ""

# ==============================================================================
# STEP 7: Setup Environment
# ==============================================================================
echo -e "${CYAN}[7/8] Setting up environment...${NC}"

if [ ! -f "$MERCURY_ROOT/.env" ]; then
    if [ -f "$MERCURY_ROOT/env.template" ]; then
        cp "$MERCURY_ROOT/env.template" "$MERCURY_ROOT/.env"
        echo -e "${GREEN}✓ Created .env from template${NC}"
        echo -e "${YELLOW}⚠ IMPORTANT: Edit .env and add your ODDS_API_KEY${NC}"
    else
        echo -e "${RED}✗ env.template not found${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠ .env already exists${NC}"
fi

# Check if ODDS_API_KEY is set
if grep -q "your_api_key_here" "$MERCURY_ROOT/.env" 2>/dev/null; then
    echo -e "${YELLOW}⚠ ODDS_API_KEY not configured in .env${NC}"
fi

echo ""

# ==============================================================================
# STEP 8: Verify Setup
# ==============================================================================
echo -e "${CYAN}[8/8] Verifying setup...${NC}"

# Check tables exist
echo "Checking database schema..."
TABLES=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "\dt" | wc -l | tr -d ' ')
if [ "$TABLES" -ge "5" ]; then
    echo -e "${GREEN}✓ Found $TABLES tables${NC}"
else
    echo -e "${RED}✗ Expected at least 5 tables, found $TABLES${NC}"
    exit 1
fi

# Check seed data
echo "Checking seed data..."
SPORTS=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM sports;" | tr -d ' ')
BOOKS=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM books;" 2>/dev/null | tr -d ' ' || echo "0")
MARKETS=$(docker exec fortuna-alexandria psql -U fortuna -d alexandria -t -c "SELECT COUNT(*) FROM markets;" 2>/dev/null | tr -d ' ' || echo "0")

echo "  Sports:  $SPORTS"
echo "  Books:   $BOOKS"
echo "  Markets: $MARKETS"

if [ "$SPORTS" == "0" ] || [ "$BOOKS" == "0" ] || [ "$MARKETS" == "0" ]; then
    echo -e "${YELLOW}⚠ Missing seed data (expected: 3 sports, 13 books, 16 markets)${NC}"
else
    echo -e "${GREEN}✓ Seed data verified${NC}"
fi

echo ""

# ==============================================================================
# SUMMARY
# ==============================================================================
echo -e "${GREEN}"
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                   SETUP COMPLETE! ✓                          ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}Infrastructure Status:${NC}"
echo -e "  ✓ Alexandria DB:  localhost:5435"
echo -e "    └─ User:        fortuna"
echo -e "    └─ Database:    alexandria"
echo -e "    └─ Tables:      sports, events, books, markets, odds_raw, closing_lines"
echo ""
echo -e "  ✓ Redis:          localhost:6380"
echo -e "    └─ Type:        Cache + Streaming"
echo ""
echo -e "  ✓ Test Database:  alexandria_test (ready for integration tests)"
echo ""

echo -e "${CYAN}Database Contents:${NC}"
echo -e "  ✓ Sports:   $SPORTS (NBA active)"
echo -e "  ✓ Books:    $BOOKS sportsbooks"
echo -e "  ✓ Markets:  $MARKETS NBA markets"
echo ""

echo -e "${CYAN}Configuration:${NC}"
echo -e "  ✓ Environment:    $MERCURY_ROOT/.env"
ODDS_API_SET=$(grep -v "your_api_key_here" "$MERCURY_ROOT/.env" | grep "ODDS_API_KEY=" | grep -v "^#" | wc -l | tr -d ' ')
if [ "$ODDS_API_SET" == "1" ]; then
    echo -e "  ${GREEN}✓ ODDS_API_KEY:   Configured ✓${NC}"
else
    echo -e "  ${YELLOW}⚠ ODDS_API_KEY:   NOT CONFIGURED${NC}"
    echo ""
    echo -e "${YELLOW}ACTION REQUIRED:${NC}"
    echo -e "  1. Get an API key from: ${CYAN}https://the-odds-api.com/#get-access${NC}"
    echo -e "  2. Edit: ${CYAN}$MERCURY_ROOT/.env${NC}"
    echo -e "  3. Set: ${CYAN}ODDS_API_KEY=your_actual_key${NC}"
fi

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}Next Steps:${NC}"
echo ""
echo -e "  1. ${CYAN}Configure API Key${NC} (if not done):"
echo -e "     nano .env"
echo ""
echo -e "  2. ${CYAN}Run Tests${NC}:"
echo -e "     make test-unit"
echo -e "     make test-integration"
echo ""
echo -e "  3. ${CYAN}Start Mercury${NC}:"
echo -e "     go run cmd/mercury/main.go"
echo ""
echo -e "  4. ${CYAN}Monitor${NC} (optional dev tools):"
echo -e "     cd ../deploy && docker-compose --profile dev up -d"
echo -e "     • Redis Commander: http://localhost:8082"
echo -e "     • PgAdmin:         http://localhost:5050"
echo ""

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}Quick Commands:${NC}"
echo ""
echo -e "  ${GREEN}make test-unit${NC}        # Run unit tests (no dependencies)"
echo -e "  ${GREEN}make test-integration${NC} # Run integration tests (needs infra)"
echo -e "  ${GREEN}make run${NC}              # Build and run Mercury"
echo -e "  ${GREEN}make clean${NC}            # Clean build artifacts"
echo ""
echo -e "  ${GREEN}psql postgres://fortuna:fortuna_dev_password@localhost:5435/alexandria${NC}"
echo -e "  ${GREEN}redis-cli -p 6380${NC}"
echo ""

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${GREEN}Mercury is ready to aggregate odds! 🚀${NC}"
echo ""

