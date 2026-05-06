#!/bin/bash
# Development tmux session setup for Applause Whisper
# Usage: ./scripts/dev-tmux.sh

SESSION="applause"
PROJECT_DIR="/Users/nnk/Desktop/work/applause/applause-whisper"

# Kill existing session if it exists
tmux kill-session -t $SESSION 2>/dev/null

# Create new session with opencode window (window 0 - reserved for AI coding)
tmux new-session -d -s $SESSION -n opencode -c "$PROJECT_DIR"

# Create wails window (window 1 - wails dev server)
tmux new-window -t $SESSION -n wails -c "$PROJECT_DIR"

# Create shell window (window 2 - general commands)
tmux new-window -t $SESSION -n shell -c "$PROJECT_DIR"

# Start wails dev in the wails window
tmux send-keys -t $SESSION:wails "wails dev" Enter

# Select the wails window by default
tmux select-window -t $SESSION:wails

echo "TMux session '$SESSION' created with windows:"
echo "  0: opencode - Reserved for opencode/AI coding"
echo "  1: wails    - Running 'wails dev'"
echo "  2: shell    - General shell commands"
echo ""
echo "Attach with: tmux attach -t $SESSION"
