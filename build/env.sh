#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
rlzdir="$workspace/src/github.com/relianz2019"
if [ ! -L "$rlzdir/rlz" ]; then
    mkdir -p "$rlzdir"
    cd "$rlzdir"
    ln -s ../../../../../. rlz
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$rlzdir/rlz"
PWD="$rlzdir/rlz"

# Launch the arguments with the configured environment.
exec "$@"
