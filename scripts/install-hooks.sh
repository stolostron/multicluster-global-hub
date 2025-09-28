#!/bin/bash

# Install git hooks for release checklist
echo "Installing git hooks..."

# Copy pre-push hook
cp scripts/hooks/pre-push .git/hooks/pre-push
chmod +x .git/hooks/pre-push

echo "✅ Git hooks installed successfully!"
echo "The pre-push hook will now remind you about the release checklist when pushing to release or main branches."