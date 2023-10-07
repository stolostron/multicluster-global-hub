#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/.." ; pwd -P)"

output=${REPO_DIR}/output
mkdir -p ${output}

# Function to start the backend application
start_backend() {
    echo "Starting the backend counter..."
    python3 ${REPO_DIR}/src/counter.py override 2>&1 > ${output}/counter.log &
}

# Function to start the backend application
continue_backend() {
    echo "Continue the backend counter..."
    pkill -f ${REPO_DIR}/src/counter.py
    python3 ${REPO_DIR}/src/counter.py 2>&1 >> ${output}/counter.log &
}

# Function to stop the backend application
stop_backend() {
    echo "Stopping the backend counter..."
    # Replace the following line with the actual command or process name to stop your backend app
    pkill -f ${REPO_DIR}/src/counter.py
    python3 ${REPO_DIR}/src/counter.py draw
}

csv_draw() {
    echo "Drawing from the csv..."
    python3 ${REPO_DIR}/src/counter.py draw
}

# Check if an argument is provided (start or stop)
if [ $# -eq 0 ]; then
    echo "Usage: $0 {start|stop}"
    exit 1
fi

# Check the argument and call the appropriate function
case "$1" in
    "start")
        start_backend
        ;;
    "continue")
        continue_backend
        ;;
    "stop")
        stop_backend
        ;;
     "draw")
        csv_draw
        ;;
    *)
        echo "Usage: $0 {start|continue|stop|draw}"
        exit 1
        ;;
esac

exit 0