#!/bin/bash

# Change directory
cd services/server/port

# Build the Go program
go build -o ../../../hornet-storage

# Pause equivalent (wait for user input)
read -p "Press enter to continue"