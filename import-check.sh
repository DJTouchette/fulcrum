# Step 1: Find the exact imports causing the cycle
echo "=== Checking imports in each package ==="

echo "1. fulcrum/cmd imports:"
grep -r "^import" cmd/ --include="*.go" | head -10

echo -e "\n2. fulcrum/lib/database imports:"
grep -r "^import\|\"fulcrum" lib/database/*.go 2>/dev/null || echo "No direct imports found"

echo -e "\n3. fulcrum/lib/database/drivers imports:"
grep -r "^import\|\"fulcrum" lib/database/drivers/*.go 2>/dev/null || echo "No drivers directory"

echo -e "\n=== Looking for specific problematic imports ==="

echo "4. Files importing fulcrum/lib/database:"
grep -r "fulcrum/lib/database" . --include="*.go" | grep -v ".git"

echo -e "\n5. Files importing fulcrum/cmd:"
grep -r "fulcrum/cmd" . --include="*.go" | grep -v ".git"

echo -e "\n=== Check for the cycle breaker ==="
echo "6. What does mysql.go import?"
find . -name "mysql.go" -exec grep -H "import\|\"fulcrum" {} \;

echo -e "\n7. What does manager.go import?"
find . -name "manager.go" -exec grep -H "import\|\"fulcrum" {} \;
